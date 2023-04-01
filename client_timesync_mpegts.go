package gohlslib

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
)

type clientTimeSyncMPEGTS struct {
	startRTC time.Time
	td       *mpegts.TimeDecoder
	mutex    sync.Mutex
}

func newClientTimeSyncMPEGTS(startDTS int64) *clientTimeSyncMPEGTS {
	return &clientTimeSyncMPEGTS{
		startRTC: time.Now(),
		td:       mpegts.NewTimeDecoder(startDTS),
	}
}

func (ts *clientTimeSyncMPEGTS) convertAndSync(ctx context.Context, rawDTS int64, rawPTS int64) (time.Duration, error) {
	ts.mutex.Lock()
	dts := ts.td.Decode(rawDTS)
	pts := ts.td.Decode(rawPTS)
	ts.mutex.Unlock()

	elapsed := time.Since(ts.startRTC)
	if dts > elapsed {
		diff := dts - elapsed
		if diff > clientMaxDTSRTCDiff {
			return 0, fmt.Errorf("difference between DTS and RTC is too big")
		}

		select {
		case <-time.After(diff):
		case <-ctx.Done():
			return 0, fmt.Errorf("terminated")
		}
	}

	return pts, nil
}
