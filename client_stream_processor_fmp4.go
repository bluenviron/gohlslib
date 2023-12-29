package gohlslib

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"

	"github.com/bluenviron/gohlslib/pkg/codecs"
)

func fmp4PickLeadingTrack(init *fmp4.Init) int {
	// pick first video track
	for _, track := range init.Tracks {
		if track.Codec.IsVideo() {
			return track.ID
		}
	}

	// otherwise, pick first track
	return init.Tracks[0].ID
}

func findPartTrackOfLeadingTrack(parts []*fmp4.Part, leadingTrackID int) *fmp4.PartTrack {
	for _, part := range parts {
		for _, partTrack := range part.Tracks {
			if partTrack.ID == leadingTrackID {
				return partTrack
			}
		}
	}
	return nil
}

func findTimeScaleOfLeadingTrack(tracks []*fmp4.InitTrack, leadingTrackID int) uint32 {
	for _, track := range tracks {
		if track.ID == leadingTrackID {
			return track.TimeScale
		}
	}
	return 0
}

type clientStreamProcessorFMP4 struct {
	ctx                  context.Context
	isLeading            bool
	initFile             []byte
	segmentQueue         *clientSegmentQueue
	rp                   *clientRoutinePool
	onStreamTracks       clientOnStreamTracksFunc
	onSetLeadingTimeSync func(clientTimeSync)
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool)
	onData               map[*Track]interface{}

	tracks             []*Track
	init               fmp4.Init
	leadingTrackID     int
	prePreProcessFuncs map[int]func(context.Context, *fmp4.PartTrack) error

	// in
	chPartTrackProcessed chan struct{}
}

func (p *clientStreamProcessorFMP4) initialize() error {
	p.chPartTrackProcessed = make(chan struct{}, clientFMP4MaxPartTracksPerSegment)

	err := p.init.Unmarshal(bytes.NewReader(p.initFile))
	if err != nil {
		return err
	}

	p.leadingTrackID = fmp4PickLeadingTrack(&p.init)

	p.tracks = make([]*Track, len(p.init.Tracks))
	for i, track := range p.init.Tracks {
		p.tracks[i] = &Track{
			Codec: codecs.FromFMP4(track.Codec),
		}
	}

	ok := p.onStreamTracks(p.ctx, p)
	if !ok {
		return fmt.Errorf("terminated")
	}

	return nil
}

func (p *clientStreamProcessorFMP4) getIsLeading() bool {
	return p.isLeading
}

func (p *clientStreamProcessorFMP4) getTracks() []*Track {
	return p.tracks
}

func (p *clientStreamProcessorFMP4) run(ctx context.Context) error {
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

func (p *clientStreamProcessorFMP4) processSegment(ctx context.Context, byts []byte) error {
	var parts fmp4.Parts
	err := parts.Unmarshal(byts)
	if err != nil {
		return err
	}

	if p.prePreProcessFuncs == nil {
		err := p.initializeTrackProcessors(ctx, parts)
		if err != nil {
			return err
		}
	}

	processingCount := 0

	for _, part := range parts {
		for _, partTrack := range part.Tracks {
			if processingCount >= (clientFMP4MaxPartTracksPerSegment - 1) {
				return fmt.Errorf("too many part tracks at once")
			}

			prePreProcess, ok := p.prePreProcessFuncs[partTrack.ID]
			if !ok {
				continue
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
		case <-p.chPartTrackProcessed:
		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	return nil
}

func (p *clientStreamProcessorFMP4) onPartTrackProcessed(ctx context.Context) {
	select {
	case p.chPartTrackProcessed <- struct{}{}:
	case <-ctx.Done():
	}
}

func (p *clientStreamProcessorFMP4) initializeTrackProcessors(
	ctx context.Context,
	parts []*fmp4.Part,
) error {
	var timeSync *clientTimeSyncFMP4

	if p.isLeading {
		trackPart := findPartTrackOfLeadingTrack(parts, p.leadingTrackID)
		if trackPart == nil {
			return nil
		}

		timeScale := findTimeScaleOfLeadingTrack(p.init.Tracks, p.leadingTrackID)

		timeSync = &clientTimeSyncFMP4{
			timeScale: timeScale,
			baseTime:  trackPart.BaseTime,
		}
		timeSync.initialize()

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
			var onDataCasted ClientOnDataAV1Func = func(pts time.Duration, tu [][]byte) {}
			if onData != nil {
				onDataCasted = onData.(ClientOnDataAV1Func)
			}

			postProcess = func(pts time.Duration, dts time.Duration, sample *fmp4.PartSample) error {
				tu, err := sample.GetAV1()
				if err != nil {
					return err
				}

				onDataCasted(pts, tu)
				return nil
			}

		case *codecs.VP9:
			var onDataCasted ClientOnDataVP9Func = func(pts time.Duration, frame []byte) {}
			if onData != nil {
				onDataCasted = onData.(ClientOnDataVP9Func)
			}

			postProcess = func(pts time.Duration, dts time.Duration, sample *fmp4.PartSample) error {
				onDataCasted(pts, sample.Payload)
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
				onDataCasted(pts, [][]byte{sample.Payload})
				return nil
			}

		case *codecs.MPEG4Audio:
			var onDataCasted ClientOnDataMPEG4AudioFunc = func(pts time.Duration, aus [][]byte) {}
			if onData != nil {
				onDataCasted = onData.(ClientOnDataMPEG4AudioFunc)
			}

			postProcess = func(pts time.Duration, dts time.Duration, sample *fmp4.PartSample) error {
				onDataCasted(pts, [][]byte{sample.Payload})
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

		trackProc := &clientTrackProcessor{}
		trackProc.initialize()
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
