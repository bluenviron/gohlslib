package gohlslib

import (
	"bytes"
	"context"
	"fmt"

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

	tracks          []*Track
	init            fmp4.Init
	leadingTrackID  int
	trackProcessors map[int]*clientTrackProcessorFMP4

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

	if p.trackProcessors == nil {
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

			trackProc, ok := p.trackProcessors[partTrack.ID]
			if !ok {
				continue
			}

			err := trackProc.push(ctx, partTrack)
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

	p.trackProcessors = make(map[int]*clientTrackProcessorFMP4)

	for i, track := range p.tracks {
		trackProc := &clientTrackProcessorFMP4{
			track:                track,
			onData:               p.onData[track],
			timeScale:            p.init.Tracks[i].TimeScale,
			timeSync:             timeSync,
			onPartTrackProcessed: p.onPartTrackProcessed,
		}
		err := trackProc.initialize()
		if err != nil {
			return err
		}
		p.rp.add(trackProc)

		p.trackProcessors[p.init.Tracks[i].ID] = trackProc
	}

	return nil
}
