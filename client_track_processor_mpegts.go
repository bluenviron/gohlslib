package gohlslib

import (
	"context"
	"fmt"
	"time"

	"github.com/bluenviron/gohlslib/pkg/codecs"
)

type mpegtsSample struct {
	pts  time.Duration
	dts  time.Duration
	data [][]byte
}

type clientTrackProcessorMPEGTS struct {
	track               *Track
	onData              interface{}
	timeSync            *clientTimeSyncMPEGTS
	onPartProcessorDone func(ctx context.Context)

	postProcess func(sample *mpegtsSample)

	queue chan *mpegtsSample
}

func (t *clientTrackProcessorMPEGTS) initialize() {
	switch t.track.Codec.(type) {
	case *codecs.H264:
		var onDataCasted ClientOnDataH26xFunc = func(pts time.Duration, dts time.Duration, au [][]byte) {}
		if t.onData != nil {
			onDataCasted = t.onData.(ClientOnDataH26xFunc)
		}

		t.postProcess = func(sample *mpegtsSample) {
			onDataCasted(sample.pts, sample.dts, sample.data)
		}

	case *codecs.MPEG4Audio:
		var onDataCasted ClientOnDataMPEG4AudioFunc = func(pts time.Duration, aus [][]byte) {}
		if t.onData != nil {
			onDataCasted = t.onData.(ClientOnDataMPEG4AudioFunc)
		}

		t.postProcess = func(sample *mpegtsSample) {
			onDataCasted(sample.pts, sample.data)
		}
	}

	t.queue = make(chan *mpegtsSample, clientMPEGTSSampleQueueSize)
}

func (t *clientTrackProcessorMPEGTS) run(ctx context.Context) error {
	for {
		select {
		case sample := <-t.queue:
			err := t.process(ctx, sample)
			if err != nil {
				return err
			}

		case <-ctx.Done():
			return nil
		}
	}
}

func (t *clientTrackProcessorMPEGTS) process(ctx context.Context, sample *mpegtsSample) error {
	if sample == nil {
		t.onPartProcessorDone(ctx)
		return nil
	}

	// silently discard packets prior to the first packet of the leading track
	if sample.pts < 0 {
		return nil
	}

	err := t.timeSync.sync(ctx, sample.dts)
	if err != nil {
		return err
	}

	t.postProcess(sample)
	return nil
}

func (t *clientTrackProcessorMPEGTS) push(ctx context.Context, sample *mpegtsSample) error {
	select {
	case t.queue <- sample:
		return nil

	case <-ctx.Done():
		return fmt.Errorf("terminated")
	}
}
