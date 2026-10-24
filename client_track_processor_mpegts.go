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
	onPartProcessorDone(errGroupCtx context.Context)
}

type clientTrackProcessorMPEGTS struct {
	track           *clientTrack
	streamProcessor clientTrackProcessorMPEGTSStreamProcessor

	queue chan *procEntryMPEGTS
}

func (t *clientTrackProcessorMPEGTS) initialize() {
	t.queue = make(chan *procEntryMPEGTS, clientMPEGTSSampleQueueSize)
}

func (t *clientTrackProcessorMPEGTS) run(errGroupCtx context.Context) error {
	for {
		select {
		case entry := <-t.queue:
			err := t.process(errGroupCtx, entry)
			if err != nil {
				return err
			}

		case <-errGroupCtx.Done():
			return nil
		}
	}
}

func (t *clientTrackProcessorMPEGTS) process(errGroupCtx context.Context, entry *procEntryMPEGTS) error {
	if entry == nil {
		t.streamProcessor.onPartProcessorDone(errGroupCtx)
		return nil
	}

	return t.track.handleData(errGroupCtx, entry.pts, entry.dts, entry.ntp, entry.data)
}

func (t *clientTrackProcessorMPEGTS) push(errGroupCtx context.Context, entry *procEntryMPEGTS) error {
	select {
	case t.queue <- entry:
		return nil

	case <-errGroupCtx.Done():
		return fmt.Errorf("terminated")
	}
}
