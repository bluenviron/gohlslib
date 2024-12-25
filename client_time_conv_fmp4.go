package gohlslib

import (
	"context"
	"sync"
	"time"
)

type clientTimeConvFMP4 struct {
	leadingTimeScale int64
	leadingBaseTime  int64

	mutex        sync.Mutex
	ntpAvailable bool
	ntpValue     time.Time
	ntpTimestamp int64
	ntpClockRate int

	chLeadingNTPReceived chan struct{}
}

func (ts *clientTimeConvFMP4) initialize() {
	ts.chLeadingNTPReceived = make(chan struct{})
}

func (ts *clientTimeConvFMP4) convert(v int64, clockRate int) int64 {
	return v - multiplyAndDivide(ts.leadingBaseTime, int64(clockRate), ts.leadingTimeScale)
}

func (ts *clientTimeConvFMP4) setNTP(value time.Time, timestamp int64, clockRate int) {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()

	ts.ntpAvailable = true
	ts.ntpValue = value
	ts.ntpTimestamp = timestamp
	ts.ntpClockRate = clockRate
}

func (ts *clientTimeConvFMP4) setLeadingNTPReceived() {
	select {
	case <-ts.chLeadingNTPReceived:
		return
	default:
	}
	close(ts.chLeadingNTPReceived)
}

func (ts *clientTimeConvFMP4) getNTP(ctx context.Context, timestamp int64, clockRate int) *time.Time {
	select {
	case <-ts.chLeadingNTPReceived:
	case <-ctx.Done():
		return nil
	}

	ts.mutex.Lock()
	defer ts.mutex.Unlock()

	if !ts.ntpAvailable {
		return nil
	}

	v := ts.ntpValue.Add(
		timestampToDuration(
			timestamp-multiplyAndDivide(ts.ntpTimestamp, int64(clockRate), int64(ts.ntpClockRate)),
			clockRate))

	return &v
}
