package gohlslib

import (
	"context"
	"fmt"
	"time"
)

func durationGoToMp4(v time.Duration, timeScale uint32) uint64 {
	timeScale64 := uint64(timeScale)
	secs := v / time.Second
	dec := v % time.Second
	return uint64(secs)*timeScale64 + uint64(dec)*timeScale64/uint64(time.Second)
}

func durationMp4ToGo(v uint64, timeScale uint32) time.Duration {
	timeScale64 := uint64(timeScale)
	secs := v / timeScale64
	dec := v % timeScale64
	return time.Duration(secs)*time.Second + time.Duration(dec)*time.Second/time.Duration(timeScale64)
}

type clientTimeSyncFMP4 struct {
	timeScale uint32
	baseTime  uint64

	startRTC time.Time
	startDTS time.Duration
}

func (ts *clientTimeSyncFMP4) initialize() {
	ts.startRTC = time.Now()
	ts.startDTS = durationMp4ToGo(ts.baseTime, ts.timeScale)
}

func (ts *clientTimeSyncFMP4) convertAndSync(ctx context.Context, timeScale uint32,
	rawDTS uint64, ptsOffset int32,
) (time.Duration, time.Duration, error) {
	pts := durationMp4ToGo(rawDTS+uint64(ptsOffset), timeScale)
	dts := durationMp4ToGo(rawDTS, timeScale)

	pts -= ts.startDTS
	dts -= ts.startDTS

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
