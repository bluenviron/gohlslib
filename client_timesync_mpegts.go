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

func (ts *clientTimeSyncMPEGTS) convertAndSync(ctx context.Context,
	rawPTS int64, rawDTS int64,
) (time.Duration, time.Duration, error) {
	ts.mutex.Lock()
	pts := ts.td.Decode(rawPTS)
	dts := ts.td.Decode(rawDTS)
	ts.mutex.Unlock()

	elapsed := time.Since(ts.startRTC)
	if dts > elapsed {
		diff := dts - elapsed
		if diff > clientMaxDTSRTCDiff {
			return 0, 0, fmt.Errorf("difference between DTS and RTC is too big")
		}

		select {
		case <-time.After(diff):
		case <-ctx.Done():
			return 0, 0, fmt.Errorf("terminated")
		}
	}

	return pts, dts, nil
}
