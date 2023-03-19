package gohlslib

import (
	"context"
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h264"
	"github.com/aler9/gortsplib/v2/pkg/format"

	"github.com/bluenviron/gohlslib/pkg/fmp4"
)

func fmp4PickLeadingTrack(init *fmp4.Init) int {
	// pick first video track
	for _, track := range init.Tracks {
		switch track.Format.(type) {
		case *format.H264, *format.H265:
			return track.ID
		}
	}

	// otherwise, pick first track
	return init.Tracks[0].ID
}

type clientProcessorFMP4 struct {
	isLeading            bool
	segmentQueue         *clientSegmentQueue
	log                  LogFunc
	rp                   *clientRoutinePool
	onSetLeadingTimeSync func(clientTimeSync)
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool)
	onData               map[format.Format]func(time.Duration, interface{})

	init           fmp4.Init
	leadingTrackID int
	trackProcs     map[int]*clientProcessorFMP4Track

	// in
	subpartProcessed chan struct{}
}

func newClientProcessorFMP4(
	ctx context.Context,
	isLeading bool,
	initFile []byte,
	segmentQueue *clientSegmentQueue,
	log LogFunc,
	rp *clientRoutinePool,
	onStreamFormats func(context.Context, []format.Format) bool,
	onSetLeadingTimeSync func(clientTimeSync),
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool),
	onData map[format.Format]func(time.Duration, interface{}),
) (*clientProcessorFMP4, error) {
	p := &clientProcessorFMP4{
		isLeading:            isLeading,
		segmentQueue:         segmentQueue,
		log:                  log,
		rp:                   rp,
		onSetLeadingTimeSync: onSetLeadingTimeSync,
		onGetLeadingTimeSync: onGetLeadingTimeSync,
		onData:               onData,
		subpartProcessed:     make(chan struct{}, clientFMP4MaxPartTracksPerSegment),
	}

	err := p.init.Unmarshal(initFile)
	if err != nil {
		return nil, err
	}

	p.leadingTrackID = fmp4PickLeadingTrack(&p.init)

	tracks := make([]format.Format, len(p.init.Tracks))
	for i, track := range p.init.Tracks {
		tracks[i] = track.Format
	}

	ok := onStreamFormats(ctx, tracks)
	if !ok {
		return nil, fmt.Errorf("terminated")
	}

	return p, nil
}

func (p *clientProcessorFMP4) run(ctx context.Context) error {
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

func (p *clientProcessorFMP4) processSegment(ctx context.Context, byts []byte) error {
	var parts fmp4.Parts
	err := parts.Unmarshal(byts)
	if err != nil {
		return err
	}

	processingCount := 0

	for _, part := range parts {
		for _, track := range part.Tracks {
			if p.trackProcs == nil {
				var ts *clientTimeSyncFMP4

				if p.isLeading {
					if track.ID != p.leadingTrackID {
						continue
					}

					timeScale := func() uint32 {
						for _, track := range p.init.Tracks {
							if track.ID == p.leadingTrackID {
								return track.TimeScale
							}
						}
						return 0
					}()
					ts = newClientTimeSyncFMP4(timeScale, track.BaseTime)
					p.onSetLeadingTimeSync(ts)
				} else {
					rawTS, ok := p.onGetLeadingTimeSync(ctx)
					if !ok {
						return fmt.Errorf("terminated")
					}

					ts, ok = rawTS.(*clientTimeSyncFMP4)
					if !ok {
						return fmt.Errorf("stream playlists are mixed MPEGTS/FMP4")
					}
				}

				p.initializeTrackProcs(ts)
			}

			proc, ok := p.trackProcs[track.ID]
			if !ok {
				continue
			}

			if processingCount >= (clientFMP4MaxPartTracksPerSegment - 1) {
				return fmt.Errorf("too many part tracks at once")
			}

			select {
			case proc.queue <- track:
			case <-ctx.Done():
				return fmt.Errorf("terminated")
			}
			processingCount++
		}
	}

	for i := 0; i < processingCount; i++ {
		select {
		case <-p.subpartProcessed:
		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	return nil
}

func (p *clientProcessorFMP4) onPartTrackProcessed(ctx context.Context) {
	select {
	case p.subpartProcessed <- struct{}{}:
	case <-ctx.Done():
	}
}

func (p *clientProcessorFMP4) initializeTrackProcs(ts *clientTimeSyncFMP4) {
	p.trackProcs = make(map[int]*clientProcessorFMP4Track)

	for _, track := range p.init.Tracks {
		var cb func(time.Duration, []byte) error

		cb2, ok := p.onData[track.Format]
		if !ok {
			cb2 = func(time.Duration, interface{}) {
			}
		}

		switch track.Format.(type) {
		case *format.H264, *format.H265:
			cb = func(pts time.Duration, payload []byte) error {
				nalus, err := h264.AVCCUnmarshal(payload)
				if err != nil {
					return err
				}

				cb2(pts, nalus)
				return nil
			}

		case *format.MPEG4Audio, *format.Opus:
			cb = func(pts time.Duration, payload []byte) error {
				cb2(pts, payload)
				return nil
			}
		}

		proc := newClientProcessorFMP4Track(
			track.TimeScale,
			ts,
			p.onPartTrackProcessed,
			cb,
		)
		p.rp.add(proc)
		p.trackProcs[track.ID] = proc
	}
}
