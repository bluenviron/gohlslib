package gohlslib

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/asticode/go-astits"

	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"

	"github.com/bluenviron/gohlslib/pkg/codecs"
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
	onDecodeError        ClientOnDecodeErrorFunc
	isLeading            bool
	segmentQueue         *clientSegmentQueue
	rp                   *clientRoutinePool
	onStreamTracks       clientOnStreamTracksFunc
	onSetLeadingTimeSync func(clientTimeSync)
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool)
	onData               map[*Track]interface{}

	switchableReader  *switchableReader
	reader            *mpegts.Reader
	tracks            []*Track
	trackProcessors   map[*Track]*clientTrackProcessorMPEGTS
	timeSync          *clientTimeSyncMPEGTS
	curSegment        *segmentData
	leadingTrackFound bool
	ntpAvailable      bool
	ntpAbsolute       time.Time
	ntpRelative       time.Duration

	chTrackProcessorDone chan struct{}
}

func (p *clientStreamProcessorMPEGTS) initialize() {
	p.chTrackProcessorDone = make(chan struct{}, clientMaxTracksPerStream)
}

func (p *clientStreamProcessorMPEGTS) getIsLeading() bool {
	return p.isLeading
}

func (p *clientStreamProcessorMPEGTS) getTracks() []*Track {
	return p.tracks
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

	for {
		err := p.reader.Read()
		if err != nil {
			if err == astits.ErrNoMorePackets {
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
	for _, track := range p.tracks {
		err := p.trackProcessors[track].push(ctx, nil)
		if err != nil {
			return err
		}
	}

	for range p.tracks {
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

func (p *clientStreamProcessorMPEGTS) initializeReader(ctx context.Context, byts []byte) error {
	p.switchableReader = &switchableReader{bytes.NewReader(byts)}

	var err error
	p.reader, err = mpegts.NewReader(p.switchableReader)
	if err != nil {
		return err
	}

	p.reader.OnDecodeError(func(err error) {
		p.onDecodeError(err)
	})

	for _, track := range p.reader.Tracks() {
		switch track.Codec.(type) {
		case *mpegts.CodecH264, *mpegts.CodecMPEG4Audio:
		default:
			return fmt.Errorf("unsupported track type: %T", track)
		}
	}

	leadingTrackID := mpegtsPickLeadingTrack(p.reader.Tracks())
	p.tracks = make([]*Track, len(p.reader.Tracks()))

	for i, mpegtsTrack := range p.reader.Tracks() {
		p.tracks[i] = &Track{
			Codec: codecs.FromMPEGTS(mpegtsTrack.Codec),
		}
	}

	if len(p.tracks) > clientMaxTracksPerStream {
		return fmt.Errorf("too many tracks per stream")
	}

	ok := p.onStreamTracks(ctx, p)
	if !ok {
		return fmt.Errorf("terminated")
	}

	for i, mpegtsTrack := range p.reader.Tracks() {
		track := p.tracks[i]
		isLeadingTrack := (i == leadingTrackID)
		var trackProc *clientTrackProcessorMPEGTS

		processSample := func(pts int64, dts int64, sample *mpegtsSample) error {
			if !p.leadingTrackFound && isLeadingTrack {
				p.leadingTrackFound = true

				if p.trackProcessors == nil {
					err := p.initializeTrackProcessors(ctx, dts)
					if err != nil {
						return err
					}
				}

				if p.curSegment.dateTime != nil {
					p.ntpAvailable = true
					p.ntpAbsolute = *p.curSegment.dateTime
					p.ntpRelative = p.timeSync.convert(dts)
				} else {
					p.ntpAvailable = false
				}
			}

			if trackProc == nil {
				trackProc = p.trackProcessors[track]

				if trackProc == nil {
					return nil
				}
			}

			return trackProc.push(ctx, sample)
		}

		switch track.Codec.(type) {
		case *codecs.H264:
			p.reader.OnDataH26x(mpegtsTrack, func(pts int64, dts int64, au [][]byte) error {
				return processSample(pts, dts, &mpegtsSample{
					pts:  pts,
					dts:  dts,
					data: au,
				})
			})

		case *codecs.MPEG4Audio:
			p.reader.OnDataMPEG4Audio(mpegtsTrack, func(pts int64, aus [][]byte) error {
				return processSample(pts, pts, &mpegtsSample{
					pts:  pts,
					dts:  pts,
					data: aus,
				})
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
		p.timeSync = &clientTimeSyncMPEGTS{
			startDTS: dts,
		}
		p.timeSync.initialize()

		p.onSetLeadingTimeSync(p.timeSync)
	} else {
		rawTS, ok := p.onGetLeadingTimeSync(ctx)
		if !ok {
			return fmt.Errorf("terminated")
		}

		p.timeSync, ok = rawTS.(*clientTimeSyncMPEGTS)
		if !ok {
			return fmt.Errorf("stream playlists are mixed MPEGTS/FMP4")
		}
	}

	p.trackProcessors = make(map[*Track]*clientTrackProcessorMPEGTS)

	for _, track := range p.tracks {
		proc := &clientTrackProcessorMPEGTS{
			track:               track,
			onData:              p.onData[track],
			timeSync:            p.timeSync,
			onPartProcessorDone: p.onPartProcessorDone,
		}
		proc.initialize()
		p.rp.add(proc)
		p.trackProcessors[track] = proc
	}

	return nil
}

func (p *clientStreamProcessorMPEGTS) ntp(dts time.Duration) (time.Time, bool) {
	if !p.ntpAvailable {
		return time.Time{}, false
	}
	return p.ntpAbsolute.Add(dts - p.ntpRelative), true
}
