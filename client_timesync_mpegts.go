package gohlslib

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
)

type clientTimeSyncMPEGTS struct {
	startDTS int64

	startRTC time.Time
	td       *mpegts.TimeDecoder
	mutex    sync.Mutex
}

func (ts *clientTimeSyncMPEGTS) initialize() {
	ts.startRTC = time.Now()
	ts.td = mpegts.NewTimeDecoder(ts.startDTS)
}

func (ts *clientTimeSyncMPEGTS) convert(v int64) time.Duration {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()
	return ts.td.Decode(v)
}

func (ts *clientTimeSyncMPEGTS) sync(
	ctx context.Context,
	dts time.Duration,
) error {
	elapsed := time.Since(ts.startRTC)

	if dts > elapsed {
		diff := dts - elapsed
		if diff > clientMaxDTSRTCDiff {
			return fmt.Errorf("difference between DTS and RTC is too big")
		}

		select {
		case <-time.After(diff):
		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	return nil
}
