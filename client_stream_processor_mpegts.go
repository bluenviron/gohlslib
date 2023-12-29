package gohlslib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/asticode/go-astits"

	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"

	"github.com/bluenviron/gohlslib/pkg/codecs"
)

var errSkipSilently = errors.New("skip silently")

func mpegtsPickLeadingTrack(mpegtsTracks []*mpegts.Track) *mpegts.Track {
	// pick first video track
	for _, track := range mpegtsTracks {
		if _, ok := track.Codec.(*mpegts.CodecH264); ok {
			return track
		}
	}

	// otherwise, pick first track
	return mpegtsTracks[0]
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

	switchableReader *switchableReader
	reader           *mpegts.Reader
	tracks           []*Track
	trackProcessors  map[*Track]*clientTrackProcessorMPEGTS
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

func (p *clientStreamProcessorMPEGTS) processSegment(ctx context.Context, byts []byte) error {
	if p.switchableReader == nil {
		err := p.initializeReader(ctx, byts)
		if err != nil {
			return err
		}
	} else {
		p.switchableReader.r = bytes.NewReader(byts)
	}

	for {
		err := p.reader.Read()
		if err != nil {
			if err == astits.ErrNoMorePackets {
				return nil
			}
			return err
		}
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

	leadingTrack := mpegtsPickLeadingTrack(p.reader.Tracks())
	p.tracks = make([]*Track, len(p.reader.Tracks()))

	for i, mpegtsTrack := range p.reader.Tracks() {
		p.tracks[i] = &Track{
			Codec: codecs.FromMPEGTS(mpegtsTrack.Codec),
		}
	}

	ok := p.onStreamTracks(ctx, p)
	if !ok {
		return fmt.Errorf("terminated")
	}

	for i, mpegtsTrack := range p.reader.Tracks() {
		track := p.tracks[i]
		isLeadingTrack := (leadingTrack == mpegtsTrack)
		var trackProc *clientTrackProcessorMPEGTS

		processSample := func(pts int64, dts int64, sample *mpegtsSample) error {
			if p.trackProcessors == nil {
				err := p.initializeTrackProcessors(ctx, isLeadingTrack, dts)
				if err != nil {
					if err == errSkipSilently {
						return nil
					}
					return err
				}
			}

			if trackProc == nil {
				trackProc = p.trackProcessors[track]
			}

			return trackProc.push(ctx, sample)
		}

		switch track.Codec.(type) {
		case *codecs.H264:
			p.reader.OnDataH26x(mpegtsTrack, func(pts int64, dts int64, au [][]byte) error {
				sample := &mpegtsSample{
					pts:  pts,
					dts:  dts,
					data: au,
				}
				return processSample(pts, dts, sample)
			})

		case *codecs.MPEG4Audio:
			p.reader.OnDataMPEG4Audio(mpegtsTrack, func(pts int64, aus [][]byte) error {
				sample := &mpegtsSample{
					pts:  pts,
					dts:  pts,
					data: aus,
				}

				return processSample(pts, pts, sample)
			})
		}
	}

	return nil
}

func (p *clientStreamProcessorMPEGTS) initializeTrackProcessors(
	ctx context.Context,
	isLeadingTrack bool,
	dts int64,
) error {
	var timeSync *clientTimeSyncMPEGTS

	if p.isLeading {
		if !isLeadingTrack {
			return errSkipSilently
		}

		timeSync = &clientTimeSyncMPEGTS{
			startDTS: dts,
		}
		timeSync.initialize()

		p.onSetLeadingTimeSync(timeSync)
	} else {
		rawTS, ok := p.onGetLeadingTimeSync(ctx)
		if !ok {
			return fmt.Errorf("terminated")
		}

		timeSync, ok = rawTS.(*clientTimeSyncMPEGTS)
		if !ok {
			return fmt.Errorf("stream playlists are mixed MPEGTS/FMP4")
		}
	}

	p.trackProcessors = make(map[*Track]*clientTrackProcessorMPEGTS)

	for _, track := range p.tracks {
		proc := &clientTrackProcessorMPEGTS{
			track:    track,
			onData:   p.onData[track],
			timeSync: timeSync,
		}
		proc.initialize()
		p.rp.add(proc)
		p.trackProcessors[track] = proc
	}

	return nil
}
