package gohlslib

import (
	"context"
	"fmt"
	"time"
)

type procEntryMPEGTS struct {
	pts  int64
	dts  int64
	ntp  *time.Time
	data [][]byte
}

type clientTrackProcessorMPEGTSStreamProcessor interface {
	onPartProcessorDone(ctx context.Context)
}
type clientTrackProcessorMPEGTS struct {
	track           *clientTrack
	streamProcessor clientTrackProcessorMPEGTSStreamProcessor

	queue chan *procEntryMPEGTS
}

func (t *clientTrackProcessorMPEGTS) initialize() {
	t.queue = make(chan *procEntryMPEGTS, clientMPEGTSSampleQueueSize)
}

func (t *clientTrackProcessorMPEGTS) run(ctx context.Context) error {
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

func (t *clientTrackProcessorMPEGTS) process(ctx context.Context, entry *procEntryMPEGTS) error {
	if entry == nil {
		t.streamProcessor.onPartProcessorDone(ctx)
		return nil
	}

	return t.track.handleData(ctx, entry.pts, entry.dts, entry.ntp, entry.data)
}

func (t *clientTrackProcessorMPEGTS) push(ctx context.Context, entry *procEntryMPEGTS) error {
	select {
	case t.queue <- entry:
		return nil

	case <-ctx.Done():
		return fmt.Errorf("terminated")
	}
}
