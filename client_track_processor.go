package gohlslib

import (
	"context"
	"fmt"
)

type clientTrackProcessor struct {
	queue chan func() error
}

func (t *clientTrackProcessor) initialize() {
	t.queue = make(chan func() error, clientMPEGTSEntryQueueSize)
}

func (t *clientTrackProcessor) run(ctx context.Context) error {
	for {
		select {
		case cb := <-t.queue:
			err := cb()
			if err != nil {
				return err
			}

		case <-ctx.Done():
			return nil
		}
	}
}

func (t *clientTrackProcessor) push(ctx context.Context, cb func() error) error {
	select {
	case t.queue <- cb:
		return nil

	case <-ctx.Done():
		return fmt.Errorf("terminated")
	}
}
