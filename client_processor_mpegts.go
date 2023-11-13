package gohlslib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/asticode/go-astits"

	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"

	"github.com/vicon-security/gohlslib/pkg/codecs"
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

type clientProcessorMPEGTS struct {
	onDecodeError        ClientOnDecodeErrorFunc
	isLeading            bool
	segmentQueue         *clientSegmentQueue
	rp                   *clientRoutinePool
	onStreamTracks       func(context.Context, []*Track) bool
	onSetLeadingTimeSync func(clientTimeSync)
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool)
	onData               map[*Track]interface{}

	switchableReader *switchableReader
	reader           *mpegts.Reader
	tracks           []*Track
	trackProcs       map[*Track]*clientTrackProcessor
	timeSync         *clientTimeSyncMPEGTS
}

func newClientProcessorMPEGTS(
	onDecodeError ClientOnDecodeErrorFunc,
	isLeading bool,
	segmentQueue *clientSegmentQueue,
	rp *clientRoutinePool,
	onStreamTracks func(context.Context, []*Track) bool,
	onSetLeadingTimeSync func(clientTimeSync),
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool),
	onData map[*Track]interface{},
) *clientProcessorMPEGTS {
	return &clientProcessorMPEGTS{
		onDecodeError:        onDecodeError,
		isLeading:            isLeading,
		segmentQueue:         segmentQueue,
		rp:                   rp,
		onStreamTracks:       onStreamTracks,
		onSetLeadingTimeSync: onSetLeadingTimeSync,
		onGetLeadingTimeSync: onGetLeadingTimeSync,
		onData:               onData,
	}
}

func (p *clientProcessorMPEGTS) run(ctx context.Context) error {
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

func (p *clientProcessorMPEGTS) processSegment(ctx context.Context, byts []byte) error {
	if p.switchableReader == nil {
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

		ok := p.onStreamTracks(ctx, p.tracks)
		if !ok {
			return fmt.Errorf("terminated")
		}

		for i, mpegtsTrack := range p.reader.Tracks() {
			track := p.tracks[i]
			isLeadingTrack := (leadingTrack == mpegtsTrack)
			var trackProc *clientTrackProcessor
			onData := p.onData[track]

			preProcess := func(ctx context.Context, rawPTS int64,
				rawDTS int64, postProcess func(time.Duration, time.Duration),
			) error {
				pts, dts, err := p.timeSync.convertAndSync(ctx, rawPTS, rawDTS)
				if err != nil {
					return err
				}

				// silently discard packets prior to the first packet of the leading track
				if pts < 0 {
					return nil
				}

				postProcess(pts, dts)
				return nil
			}

			prePreProcess := func(pts int64, dts int64, postProcess func(time.Duration, time.Duration)) error {
				err := p.initializeTrackProcs(ctx, isLeadingTrack, dts)
				if err != nil {
					if err == errSkipSilently {
						return nil
					}
					return err
				}

				if trackProc == nil {
					trackProc = p.trackProcs[track]
				}

				return trackProc.push(ctx, func() error {
					return preProcess(ctx, pts, dts, postProcess)
				})
			}

			switch track.Codec.(type) {
			case *codecs.H264:
				var onDataCasted ClientOnDataH26xFunc = func(pts time.Duration, dts time.Duration, au [][]byte) {}
				if onData != nil {
					onDataCasted = onData.(ClientOnDataH26xFunc)
				}

				p.reader.OnDataH26x(mpegtsTrack, func(pts int64, dts int64, au [][]byte) error {
					return prePreProcess(
						pts, dts,
						func(pts time.Duration, dts time.Duration) {
							onDataCasted(pts, dts, au)
						})
				})

			case *codecs.MPEG4Audio:
				var onDataCasted ClientOnDataMPEG4AudioFunc = func(pts time.Duration, aus [][]byte) {}
				if onData != nil {
					onDataCasted = onData.(ClientOnDataMPEG4AudioFunc)
				}

				p.reader.OnDataMPEG4Audio(mpegtsTrack, func(pts int64, aus [][]byte) error {
					return prePreProcess(
						pts, pts,
						func(pts time.Duration, dts time.Duration) {
							onDataCasted(pts, aus)
						})
				})
			}
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

func (p *clientProcessorMPEGTS) initializeTrackProcs(ctx context.Context, isLeadingTrack bool, dts int64) error {
	if p.trackProcs != nil {
		return nil
	}

	if p.isLeading {
		if !isLeadingTrack {
			return errSkipSilently
		}

		p.timeSync = newClientTimeSyncMPEGTS(dts)
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

	p.trackProcs = make(map[*Track]*clientTrackProcessor)

	for _, track := range p.tracks {
		proc := newClientTrackProcessor()
		p.rp.add(proc)
		p.trackProcs[track] = proc
	}

	return nil
}
