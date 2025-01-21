package gohlslib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/asticode/go-astits"

	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"

	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

func mpegtsPickLeadingTrack(mpegtsTracks []*mpegts.Track) int {
	// pick first video track
	for i, track := range mpegtsTracks {
		if _, ok := track.Codec.(*mpegts.CodecH264); ok {
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

type clientStreamProcessorMPEGTS struct {
	onDecodeError      ClientOnDecodeErrorFunc
	isLeading          bool
	segmentQueue       *clientSegmentQueue
	rp                 *clientRoutinePool
	setTracks          func(ctx context.Context, tracks []*Track) ([]*clientTrack, bool)
	setEnded           func()
	setLeadingTimeConv func(clientTimeConv)
	getLeadingTimeConv func(context.Context) (clientTimeConv, bool)

	switchableReader   *switchableReader
	reader             *mpegts.Reader
	trackProcessors    map[*Track]*clientTrackProcessorMPEGTS
	timeConv           *clientTimeConvMPEGTS
	curSegment         *segmentData
	leadingTrackFound  bool
	dateTimeProcessed  bool
	clientStreamTracks []*clientTrack

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

		err := p.processSegment(ctx, seg)
		if err != nil {
			return err
		}
	}
}

func (p *clientStreamProcessorMPEGTS) processSegment(ctx context.Context, seg *segmentData) error {
	if seg == nil {
		p.setEnded()
		<-ctx.Done()
		return fmt.Errorf("terminated")
	}

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
			if errors.Is(err, astits.ErrNoMorePackets) {
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

	var err error
	p.reader, err = mpegts.NewReader(p.switchableReader)
	if err != nil {
		return err
	}

	p.reader.OnDecodeError(func(err error) {
		p.onDecodeError(err)
	})

	var supportedTracks []*mpegts.Track

	for _, track := range p.reader.Tracks() {
		switch track.Codec.(type) {
		case *mpegts.CodecH264, *mpegts.CodecMPEG4Audio:
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
			Codec:     codecs.FromMPEGTS(mpegtsTrack.Codec),
			ClockRate: 90000,
		}
	}

	if len(tracks) > clientMaxTracksPerStream {
		return fmt.Errorf("too many tracks per stream")
	}

	var ok bool
	p.clientStreamTracks, ok = p.setTracks(ctx, tracks)
	if !ok {
		return fmt.Errorf("terminated")
	}

	for i, mpegtsTrack := range supportedTracks {
		track := p.clientStreamTracks[i]
		isLeadingTrack := (i == leadingTrackID)
		var trackProc *clientTrackProcessorMPEGTS

		processSample := func(rawPTS int64, rawDTS int64, data [][]byte) error {
			if isLeadingTrack {
				p.leadingTrackFound = true

				if p.trackProcessors == nil {
					err := p.initializeTrackProcessors(ctx, rawDTS)
					if err != nil {
						return err
					}
				}
			}

			if trackProc == nil {
				trackProc = p.trackProcessors[track.track]

				// wait leading track before proceeding
				if trackProc == nil {
					return nil
				}
			}

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

		switch track.track.Codec.(type) {
		case *codecs.H264:
			p.reader.OnDataH264(mpegtsTrack, func(pts int64, dts int64, au [][]byte) error {
				return processSample(pts, dts, au)
			})

		case *codecs.MPEG4Audio:
			p.reader.OnDataMPEG4Audio(mpegtsTrack, func(pts int64, aus [][]byte) error {
				return processSample(pts, pts, aus)
			})
		}
	}

	return nil
}

func (p *clientStreamProcessorMPEGTS) initializeTrackProcessors(
	ctx context.Context,
	dts int64,
) error {
	if p.isLeading {
		p.timeConv = &clientTimeConvMPEGTS{
			startDTS: dts,
		}
		p.timeConv.initialize()

		p.setLeadingTimeConv(p.timeConv)
	} else {
		rawTS, ok := p.getLeadingTimeConv(ctx)
		if !ok {
			return fmt.Errorf("terminated")
		}

		p.timeConv, ok = rawTS.(*clientTimeConvMPEGTS)
		if !ok {
			return fmt.Errorf("stream playlists are mixed MPEGTS/FMP4")
		}
	}

	p.trackProcessors = make(map[*Track]*clientTrackProcessorMPEGTS)

	for _, track := range p.clientStreamTracks {
		proc := &clientTrackProcessorMPEGTS{
			track:               track,
			onPartProcessorDone: p.onPartProcessorDone,
		}
		proc.initialize()
		p.rp.add(proc)
		p.trackProcessors[track.track] = proc
	}

	return nil
}
