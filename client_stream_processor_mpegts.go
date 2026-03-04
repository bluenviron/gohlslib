package gohlslib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"

	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

const (
	clientMaxQueuedSamples = 1000
)

func mpegtsPickLeadingTrack(mpegtsTracks []*mpegts.Track) int {
	// pick first video track
	for i, track := range mpegtsTracks {
		if _, ok := track.Codec.(*tscodecs.H264); ok {
			return i
		}
	}

	// otherwise, pick first track
	return 0
}

type switchableReader struct {
	r io.Reader
}

func (r *switchableReader) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

type clientStreamProcessorStreamDownloader interface {
	setTracks(ctx context.Context, tracks []*Track) ([]*clientTrack, bool)
	onProcessorError(ctx context.Context, err error)
}

type clientStreamProcessorMPEGTS struct {
	onDecodeError    ClientOnDecodeErrorFunc
	isLeading        bool
	segmentQueue     *clientSegmentQueue
	rp               *clientRoutinePool
	streamDownloader clientStreamProcessorStreamDownloader
	client           clientStreamDownloaderClient

	switchableReader  *switchableReader
	reader            *mpegts.Reader
	trackProcessors   map[*Track]*clientTrackProcessorMPEGTS
	curSegment        *segmentData
	leadingTrackFound bool
	dateTimeProcessed bool
	streamTracks      []*clientTrack
	timeConv          *clientTimeConvMPEGTS
	queuedSamples     []func() error

	chTrackProcessorDone chan struct{}
}

func (p *clientStreamProcessorMPEGTS) initialize() {
	p.chTrackProcessorDone = make(chan struct{}, clientMaxTracksPerStream)
}

func (p *clientStreamProcessorMPEGTS) run(ctx context.Context) error {
	for {
		seg, ok := p.segmentQueue.pull(ctx)
		if !ok {
			return fmt.Errorf("terminated")
		}

		if seg.err != nil {
			p.streamDownloader.onProcessorError(ctx, seg.err)
			<-ctx.Done()
			return fmt.Errorf("terminated")
		}

		err := p.processSegment(ctx, seg)
		if err != nil {
			return err
		}
	}
}

func (p *clientStreamProcessorMPEGTS) processSegment(ctx context.Context, seg *segmentData) error {
	if p.switchableReader == nil {
		err := p.initializeReader(ctx, seg.payload)
		if err != nil {
			return err
		}
	} else {
		p.switchableReader.r = bytes.NewReader(seg.payload)
	}

	p.curSegment = seg
	p.leadingTrackFound = false
	p.dateTimeProcessed = false

	for {
		err := p.reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
	}

	if !p.leadingTrackFound {
		return fmt.Errorf("could not find data of leading track")
	}

	return p.joinTrackProcessors(ctx)
}

func (p *clientStreamProcessorMPEGTS) joinTrackProcessors(ctx context.Context) error {
	for _, proc := range p.trackProcessors {
		err := proc.push(ctx, nil)
		if err != nil {
			return err
		}
	}

	for range p.trackProcessors {
		select {
		case <-p.chTrackProcessorDone:
		case <-ctx.Done():
			return nil
		}
	}

	return nil
}

func (p *clientStreamProcessorMPEGTS) onPartProcessorDone(ctx context.Context) {
	select {
	case p.chTrackProcessorDone <- struct{}{}:
	case <-ctx.Done():
	}
}

func (p *clientStreamProcessorMPEGTS) initializeReader(ctx context.Context, firstPayload []byte) error {
	p.switchableReader = &switchableReader{bytes.NewReader(firstPayload)}

	p.reader = &mpegts.Reader{R: p.switchableReader}
	err := p.reader.Initialize()
	if err != nil {
		return err
	}

	p.reader.OnDecodeError(func(err error) {
		p.onDecodeError(err)
	})

	var supportedTracks []*mpegts.Track

	for _, track := range p.reader.Tracks() {
		switch track.Codec.(type) {
		case *tscodecs.H264, *tscodecs.MPEG4Audio:
			supportedTracks = append(supportedTracks, track)
		}
	}

	if len(supportedTracks) == 0 {
		return fmt.Errorf("no supported tracks found")
	}

	leadingTrackID := mpegtsPickLeadingTrack(supportedTracks)

	tracks := make([]*Track, len(supportedTracks))

	for i, mpegtsTrack := range supportedTracks {
		tracks[i] = &Track{
			Codec:     fromMPEGTS(mpegtsTrack.Codec),
			ClockRate: 90000,
		}
	}

	if len(tracks) > clientMaxTracksPerStream {
		return fmt.Errorf("too many tracks per stream")
	}

	var ok bool
	p.streamTracks, ok = p.streamDownloader.setTracks(ctx, tracks)
	if !ok {
		return fmt.Errorf("terminated")
	}

	err = p.initializeTrackProcessors()
	if err != nil {
		return err
	}

	for i, mpegtsTrack := range supportedTracks {
		track := p.streamTracks[i]
		isLeadingTrack := (i == leadingTrackID)
		trackProc := p.trackProcessors[track.track]

		switch track.track.Codec.(type) {
		case *codecs.H264:
			p.reader.OnDataH264(mpegtsTrack, func(pts int64, dts int64, au [][]byte) error {
				return p.processSample(ctx, isLeadingTrack, trackProc, pts, dts, au)
			})

		case *codecs.MPEG4Audio:
			p.reader.OnDataMPEG4Audio(mpegtsTrack, func(pts int64, aus [][]byte) error {
				return p.processSample(ctx, isLeadingTrack, trackProc, pts, pts, aus)
			})
		}
	}

	return nil
}

func (p *clientStreamProcessorMPEGTS) processSample(
	ctx context.Context,
	isLeadingTrack bool,
	trackProc *clientTrackProcessorMPEGTS,
	rawPTS int64,
	rawDTS int64,
	data [][]byte,
) error {
	if isLeadingTrack {
		p.leadingTrackFound = true
	}

	if p.timeConv == nil {
		if isLeadingTrack {
			err := p.initializeTimeConv(ctx, rawDTS)
			if err != nil {
				return err
			}

			err = p.processSample2(ctx, isLeadingTrack, trackProc, rawPTS, rawDTS, data)
			if err != nil {
				return err
			}

			for _, call := range p.queuedSamples {
				err = call()
				if err != nil {
					return err
				}
			}
			p.queuedSamples = nil

			return nil
		}

		if len(p.queuedSamples) >= clientMaxQueuedSamples {
			return fmt.Errorf("too many queued samples")
		}

		p.queuedSamples = append(p.queuedSamples, func() error {
			return p.processSample2(ctx, isLeadingTrack, trackProc, rawPTS, rawDTS, data)
		})
		return nil
	}

	return p.processSample2(ctx, isLeadingTrack, trackProc, rawPTS, rawDTS, data)
}

func (p *clientStreamProcessorMPEGTS) processSample2(
	ctx context.Context,
	isLeadingTrack bool,
	trackProc *clientTrackProcessorMPEGTS,
	rawPTS int64,
	rawDTS int64,
	data [][]byte,
) error {
	pts := p.timeConv.convert(rawPTS)
	dts := p.timeConv.convert(rawDTS)

	if !p.dateTimeProcessed && p.isLeading && isLeadingTrack {
		p.dateTimeProcessed = true

		if p.curSegment.dateTime != nil {
			p.timeConv.setNTP(*p.curSegment.dateTime, dts)
		}
		p.timeConv.setLeadingNTPReceived()
	}

	ntp := p.timeConv.getNTP(ctx, dts)

	return trackProc.push(ctx, &procEntryMPEGTS{
		pts:  pts,
		dts:  dts,
		ntp:  ntp,
		data: data,
	})
}

func (p *clientStreamProcessorMPEGTS) initializeTrackProcessors() error {
	p.trackProcessors = make(map[*Track]*clientTrackProcessorMPEGTS)

	for _, track := range p.streamTracks {
		trackProc := &clientTrackProcessorMPEGTS{
			track:           track,
			streamProcessor: p,
		}
		trackProc.initialize()
		p.rp.add(trackProc)
		p.trackProcessors[track.track] = trackProc
	}

	return nil
}

func (p *clientStreamProcessorMPEGTS) initializeTimeConv(ctx context.Context, startDTS int64) error {
	if p.isLeading {
		p.timeConv = &clientTimeConvMPEGTS{
			startDTS: startDTS,
		}
		p.timeConv.initialize()

		p.client.setTimeConv(p.timeConv)
	} else {
		tmp, ok := p.client.waitTimeConv(ctx)
		if !ok {
			return fmt.Errorf("terminated")
		}

		p.timeConv, ok = tmp.(*clientTimeConvMPEGTS)
		if !ok {
			return fmt.Errorf("stream playlists are mixed MPEGTS/FMP4")
		}
	}

	return nil
}
