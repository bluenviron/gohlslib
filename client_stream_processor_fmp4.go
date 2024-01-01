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

func findFirstPartTrackOfLeadingTrack(parts []*fmp4.Part, leadingTrackID int) *fmp4.PartTrack {
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
	onStreamEnded        func(context.Context)
	onSetLeadingTimeSync func(clientTimeSync)
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool)
	onData               map[*Track]interface{}

	tracks          []*Track
	init            fmp4.Init
	leadingTrackID  int
	trackProcessors map[int]*clientTrackProcessorFMP4
	timeSync        *clientTimeSyncFMP4
	ntpAvailable    bool
	ntpAbsolute     time.Time
	ntpRelative     time.Duration

	// in
	chPartTrackProcessed chan struct{}
}

func (p *clientStreamProcessorFMP4) initialize() {
	p.chPartTrackProcessed = make(chan struct{}, clientMaxTracksPerStream)
}

func (p *clientStreamProcessorFMP4) getIsLeading() bool {
	return p.isLeading
}

func (p *clientStreamProcessorFMP4) getTracks() []*Track {
	return p.tracks
}

func (p *clientStreamProcessorFMP4) run(ctx context.Context) error {
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

	if len(p.tracks) > clientMaxTracksPerStream {
		return fmt.Errorf("too many tracks per stream")
	}

	ok := p.onStreamTracks(p.ctx, p)
	if !ok {
		return fmt.Errorf("terminated")
	}

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

func (p *clientStreamProcessorFMP4) processSegment(ctx context.Context, seg *segmentData) error {
	if seg == nil {
		p.onStreamEnded(ctx)
		<-ctx.Done()
		return fmt.Errorf("terminated")
	}

	var parts fmp4.Parts
	err := parts.Unmarshal(seg.payload)
	if err != nil {
		return err
	}

	if p.trackProcessors == nil || seg.dateTime != nil {
		partTrack := findFirstPartTrackOfLeadingTrack(parts, p.leadingTrackID)
		if partTrack == nil {
			return fmt.Errorf("could not find data of leading track")
		}

		if p.trackProcessors == nil {
			err := p.initializeTrackProcessors(ctx, partTrack)
			if err != nil {
				return err
			}
		}

		if seg.dateTime != nil {
			p.ntpAvailable = true
			p.ntpAbsolute = *seg.dateTime
			p.ntpRelative = p.timeSync.convert(partTrack.BaseTime, p.timeSync.leadingTimeScale)
		} else {
			p.ntpAvailable = false
		}
	}

	partTrackCount := 0

	for _, part := range parts {
		for _, partTrack := range part.Tracks {
			trackProc, ok := p.trackProcessors[partTrack.ID]
			if !ok {
				continue
			}

			err := trackProc.push(ctx, partTrack)
			if err != nil {
				return err
			}

			partTrackCount++
		}
	}

	return p.joinTrackProcessors(ctx, partTrackCount)
}

func (p *clientStreamProcessorFMP4) joinTrackProcessors(ctx context.Context, partTrackCount int) error {
	for i := 0; i < partTrackCount; i++ {
		select {
		case <-p.chPartTrackProcessed:
		case <-ctx.Done():
			return nil
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
	partTrack *fmp4.PartTrack,
) error {
	if p.isLeading {
		timeScale := findTimeScaleOfLeadingTrack(p.init.Tracks, p.leadingTrackID)

		p.timeSync = &clientTimeSyncFMP4{
			leadingTimeScale: timeScale,
			initialBaseTime:  partTrack.BaseTime,
		}
		p.timeSync.initialize()

		p.onSetLeadingTimeSync(p.timeSync)
	} else {
		rawTS, ok := p.onGetLeadingTimeSync(ctx)
		if !ok {
			return fmt.Errorf("terminated")
		}

		p.timeSync, ok = rawTS.(*clientTimeSyncFMP4)
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
			timeSync:             p.timeSync,
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

func (p *clientStreamProcessorFMP4) ntp(dts time.Duration) (time.Time, bool) {
	if !p.ntpAvailable {
		return time.Time{}, false
	}
	return p.ntpAbsolute.Add(dts - p.ntpRelative), true
}
