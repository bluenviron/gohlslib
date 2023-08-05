package gohlslib

import (
	"context"
	"fmt"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"

	"github.com/bluenviron/gohlslib/pkg/codecs"
)

func fmp4PickLeadingTrack(init *fmp4.Init) int {
	// pick first video track
	for _, track := range init.Tracks {
		switch track.Codec.(type) {
		case *fmp4.CodecAV1, *fmp4.CodecH264, *fmp4.CodecH265:
			return track.ID
		}
	}

	// otherwise, pick first track
	return init.Tracks[0].ID
}

type clientProcessorFMP4 struct {
	isLeading            bool
	segmentQueue         *clientSegmentQueue
	rp                   *clientRoutinePool
	onSetLeadingTimeSync func(clientTimeSync)
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool)
	onData               map[*Track]interface{}

	tracks             []*Track
	init               fmp4.Init
	leadingTrackID     int
	prePreProcessFuncs map[int]func(context.Context, *fmp4.PartTrack) error

	// in
	subpartProcessed chan struct{}
}

func newClientProcessorFMP4(
	ctx context.Context,
	isLeading bool,
	initFile []byte,
	segmentQueue *clientSegmentQueue,
	rp *clientRoutinePool,
	onStreamTracks func(context.Context, []*Track) bool,
	onSetLeadingTimeSync func(clientTimeSync),
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool),
	onData map[*Track]interface{},
) (*clientProcessorFMP4, error) {
	p := &clientProcessorFMP4{
		isLeading:            isLeading,
		segmentQueue:         segmentQueue,
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

	p.tracks = make([]*Track, len(p.init.Tracks))
	for i, track := range p.init.Tracks {
		p.tracks[i] = &Track{
			Codec: codecs.FromFMP4(track.Codec),
		}
	}

	ok := onStreamTracks(ctx, p.tracks)
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
		for _, partTrack := range part.Tracks {
			err := p.initializeTrackProcs(ctx, partTrack)
			if err != nil {
				if err == errSkipSilently {
					continue
				}
				return err
			}

			prePreProcess, ok := p.prePreProcessFuncs[partTrack.ID]
			if !ok {
				continue
			}

			if processingCount >= (clientFMP4MaxPartTracksPerSegment - 1) {
				return fmt.Errorf("too many part tracks at once")
			}

			err = prePreProcess(ctx, partTrack)
			if err != nil {
				return err
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

func (p *clientProcessorFMP4) initializeTrackProcs(ctx context.Context, track *fmp4.PartTrack) error {
	if p.prePreProcessFuncs != nil {
		return nil
	}

	var timeSync *clientTimeSyncFMP4
	isLeadingTrack := (track.ID == p.leadingTrackID)

	if p.isLeading {
		if !isLeadingTrack {
			return errSkipSilently
		}

		timeScale := func() uint32 {
			for _, track := range p.init.Tracks {
				if isLeadingTrack {
					return track.TimeScale
				}
			}
			return 0
		}()
		timeSync = newClientTimeSyncFMP4(timeScale, track.BaseTime)
		p.onSetLeadingTimeSync(timeSync)
	} else {
		rawTS, ok := p.onGetLeadingTimeSync(ctx)
		if !ok {
			return fmt.Errorf("terminated")
		}

		timeSync, ok = rawTS.(*clientTimeSyncFMP4)
		if !ok {
			return fmt.Errorf("stream playlists are mixed MPEG-TS/fMP4")
		}
	}

	p.prePreProcessFuncs = make(map[int]func(context.Context, *fmp4.PartTrack) error)

	for i, track := range p.tracks {
		onData := p.onData[track]

		var postProcess func(pts time.Duration, dts time.Duration, sample *fmp4.PartSample) error

		switch track.Codec.(type) {
		case *codecs.AV1:
			var onDataCasted ClientOnDataAV1Func = func(pts time.Duration, obus [][]byte) {}
			if onData != nil {
				onDataCasted = onData.(ClientOnDataAV1Func)
			}

			postProcess = func(pts time.Duration, dts time.Duration, sample *fmp4.PartSample) error {
				obus, err := sample.GetAV1()
				if err != nil {
					return err
				}

				onDataCasted(pts, obus)
				return nil
			}

		case *codecs.H265, *codecs.H264:
			var onDataCasted ClientOnDataH26xFunc = func(pts time.Duration, dts time.Duration, au [][]byte) {}
			if onData != nil {
				onDataCasted = onData.(ClientOnDataH26xFunc)
			}

			postProcess = func(pts time.Duration, dts time.Duration, sample *fmp4.PartSample) error {
				au, err := sample.GetH26x()
				if err != nil {
					return err
				}

				onDataCasted(pts, dts, au)
				return nil
			}

		case *codecs.Opus:
			var onDataCasted ClientOnDataOpusFunc = func(pts time.Duration, packets [][]byte) {}
			if onData != nil {
				onDataCasted = onData.(ClientOnDataOpusFunc)
			}

			postProcess = func(pts time.Duration, dts time.Duration, sample *fmp4.PartSample) error {
				onDataCasted(pts, [][]byte{sample.GetAudio()})
				return nil
			}

		case *codecs.MPEG4Audio:
			var onDataCasted ClientOnDataMPEG4AudioFunc = func(pts time.Duration, aus [][]byte) {}
			if onData != nil {
				onDataCasted = onData.(ClientOnDataMPEG4AudioFunc)
			}

			postProcess = func(pts time.Duration, dts time.Duration, sample *fmp4.PartSample) error {
				onDataCasted(pts, [][]byte{sample.GetAudio()})
				return nil
			}
		}

		timeScale := p.init.Tracks[i].TimeScale

		preProcess := func(ctx context.Context, partTrack *fmp4.PartTrack) error {
			rawDTS := partTrack.BaseTime

			for _, sample := range partTrack.Samples {
				pts, dts, err := timeSync.convertAndSync(ctx, timeScale, rawDTS, sample.PTSOffset)
				if err != nil {
					return err
				}

				rawDTS += uint64(sample.Duration)

				// silently discard packets prior to the first packet of the leading track
				if pts < 0 {
					continue
				}

				err = postProcess(pts, dts, sample)
				if err != nil {
					return err
				}
			}

			p.onPartTrackProcessed(ctx)
			return nil
		}

		trackProc := newClientTrackProcessor()
		p.rp.add(trackProc)

		prePreProcess := func(ctx context.Context, partTrack *fmp4.PartTrack) error {
			return trackProc.push(ctx, func() error {
				return preProcess(ctx, partTrack)
			})
		}

		p.prePreProcessFuncs[p.init.Tracks[i].ID] = prePreProcess
	}

	return nil
}
