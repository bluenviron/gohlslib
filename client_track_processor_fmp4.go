package gohlslib

import (
	"context"
	"fmt"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

type procEntryFMP4 struct {
	partTrack *fmp4.PartTrack
	dts       int64
	ntp       *time.Time
}

type clientTrackProcessorFMP4 struct {
	track                *clientTrack
	onPartTrackProcessed func(ctx context.Context)

	decodePayload func(sample *fmp4.PartSample) ([][]byte, error)

	// in
	queue chan *procEntryFMP4
}

func (t *clientTrackProcessorFMP4) initialize() error {
	switch t.track.track.Codec.(type) {
	case *codecs.AV1:
		t.decodePayload = func(sample *fmp4.PartSample) ([][]byte, error) {
			return sample.GetAV1()
		}

	case *codecs.VP9:
		t.decodePayload = func(sample *fmp4.PartSample) ([][]byte, error) {
			return [][]byte{sample.Payload}, nil
		}

	case *codecs.H264:
		t.decodePayload = func(sample *fmp4.PartSample) ([][]byte, error) {
			return sample.GetH264()
		}

	case *codecs.H265:
		t.decodePayload = func(sample *fmp4.PartSample) ([][]byte, error) {
			return sample.GetH265()
		}

	case *codecs.Opus:
		t.decodePayload = func(sample *fmp4.PartSample) ([][]byte, error) {
			return [][]byte{sample.Payload}, nil
		}

	case *codecs.MPEG4Audio:
		t.decodePayload = func(sample *fmp4.PartSample) ([][]byte, error) {
			return [][]byte{sample.Payload}, nil
		}
	}

	t.queue = make(chan *procEntryFMP4)

	return nil
}

func (t *clientTrackProcessorFMP4) run(ctx context.Context) error {
	for {
		select {
		case entry := <-t.queue:
			err := t.process(ctx, entry)
			if err != nil {
				return err
			}

		case <-ctx.Done():
			return nil
		}
	}
}

func (t *clientTrackProcessorFMP4) process(ctx context.Context, entry *procEntryFMP4) error {
	dts := entry.dts

	for _, sample := range entry.partTrack.Samples {
		data, err := t.decodePayload(sample)
		if err != nil {
			return err
		}

		pts := dts + int64(sample.PTSOffset)

		var ntp *time.Time
		if entry.ntp != nil {
			v := entry.ntp.Add(timestampToDuration(dts-entry.dts, t.track.track.ClockRate))
			ntp = &v
		}

		err = t.track.handleData(ctx, pts, dts, ntp, data)
		if err != nil {
			return err
		}

		dts += int64(sample.Duration)
	}

	t.onPartTrackProcessed(ctx)
	return nil
}

func (t *clientTrackProcessorFMP4) push(ctx context.Context, entry *procEntryFMP4) error {
	select {
	case t.queue <- entry:
		return nil

	case <-ctx.Done():
		return fmt.Errorf("terminated")
	}
}
