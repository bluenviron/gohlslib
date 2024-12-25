package gohlslib

import (
	"bytes"
	"context"
	"fmt"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"

	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/gohlslib/v2/pkg/playlist"
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
	ctx                context.Context
	isLeading          bool
	rendition          *playlist.MultivariantRendition
	initFile           []byte
	segmentQueue       *clientSegmentQueue
	rp                 *clientRoutinePool
	setTracks          func(ctx context.Context, tracks []*Track) ([]*clientTrack, bool)
	setEnded           func()
	setLeadingTimeConv func(clientTimeConv)
	getLeadingTimeConv func(context.Context) (clientTimeConv, bool)

	init               fmp4.Init
	leadingTrackID     int
	trackProcessors    map[int]*clientTrackProcessorFMP4
	timeConv           *clientTimeConvFMP4
	clientStreamTracks []*clientTrack

	// in
	chPartTrackProcessed chan struct{}
}

func (p *clientStreamProcessorFMP4) initialize() {
	p.chPartTrackProcessed = make(chan struct{}, clientMaxTracksPerStream)
}

func (p *clientStreamProcessorFMP4) run(ctx context.Context) error {
	err := p.init.Unmarshal(bytes.NewReader(p.initFile))
	if err != nil {
		return err
	}

	if !p.isLeading && len(p.init.Tracks) != 1 {
		return fmt.Errorf("rendition playlists with multiple tracks are not supported")
	}

	p.leadingTrackID = fmp4PickLeadingTrack(&p.init)

	tracks := make([]*Track, len(p.init.Tracks))

	for i, track := range p.init.Tracks {
		tracks[i] = &Track{
			Codec:     codecs.FromFMP4(track.Codec),
			ClockRate: int(track.TimeScale),
			Name: func() string {
				if !p.isLeading {
					return p.rendition.Name
				}
				return ""
			}(),
			Language: func() string {
				if !p.isLeading {
					return p.rendition.Language
				}
				return ""
			}(),
			IsDefault: func() bool {
				if !p.isLeading {
					return p.rendition.Default
				}
				return false
			}(),
		}
	}

	if len(tracks) > clientMaxTracksPerStream {
		return fmt.Errorf("too many tracks per stream")
	}

	var ok bool
	p.clientStreamTracks, ok = p.setTracks(p.ctx, tracks)
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
		p.setEnded()
		<-ctx.Done()
		return fmt.Errorf("terminated")
	}

	var parts fmp4.Parts
	err := parts.Unmarshal(seg.payload)
	if err != nil {
		return err
	}

	leadingPartTrack := findFirstPartTrackOfLeadingTrack(parts, p.leadingTrackID)
	if leadingPartTrack == nil {
		return fmt.Errorf("could not find data of leading track")
	}

	if p.trackProcessors == nil {
		err := p.initializeTrackProcessors(ctx, leadingPartTrack)
		if err != nil {
			return err
		}
	}

	if p.isLeading {
		if seg.dateTime != nil {
			leadingPartTrackProc := p.trackProcessors[leadingPartTrack.ID]
			dts := p.timeConv.convert(int64(leadingPartTrack.BaseTime), leadingPartTrackProc.track.track.ClockRate)
			p.timeConv.setNTP(*seg.dateTime, dts, leadingPartTrackProc.track.track.ClockRate)
		}
		p.timeConv.setLeadingNTPReceived()
	}

	partTrackCount := 0

	for _, part := range parts {
		for _, partTrack := range part.Tracks {
			trackProc, ok := p.trackProcessors[partTrack.ID]
			if !ok {
				continue
			}

			dts := p.timeConv.convert(int64(partTrack.BaseTime), trackProc.track.track.ClockRate)
			ntp := p.timeConv.getNTP(ctx, dts, trackProc.track.track.ClockRate)

			err := trackProc.push(ctx, &procEntryFMP4{
				partTrack: partTrack,
				dts:       dts,
				ntp:       ntp,
			})
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

		p.timeConv = &clientTimeConvFMP4{
			leadingTimeScale: int64(timeScale),
			leadingBaseTime:  int64(partTrack.BaseTime),
		}
		p.timeConv.initialize()

		p.setLeadingTimeConv(p.timeConv)
	} else {
		rawTS, ok := p.getLeadingTimeConv(ctx)
		if !ok {
			return fmt.Errorf("terminated")
		}

		p.timeConv, ok = rawTS.(*clientTimeConvFMP4)
		if !ok {
			return fmt.Errorf("stream playlists are mixed MPEG-TS/fMP4")
		}
	}

	p.trackProcessors = make(map[int]*clientTrackProcessorFMP4)

	for i, track := range p.clientStreamTracks {
		trackProc := &clientTrackProcessorFMP4{
			track:                track,
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
