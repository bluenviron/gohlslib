package gohlslib

import (
	"context"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
)

type clientTimeConvMPEGTS struct {
	startDTS int64

	mutex        sync.Mutex
	td           *mpegts.TimeDecoder
	ntpAvailable bool
	ntpValue     time.Time
	ntpTimestamp int64

	chLeadingNTPReceived chan struct{}
}

func (ts *clientTimeConvMPEGTS) initialize() {
	ts.td = &mpegts.TimeDecoder{}
	ts.td.Initialize()
	ts.td.Decode(ts.startDTS)
	ts.chLeadingNTPReceived = make(chan struct{})
}

func (ts *clientTimeConvMPEGTS) convert(v int64) int64 {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()

	return ts.td.Decode(v)
}

func (ts *clientTimeConvMPEGTS) setNTP(value time.Time, timestamp int64) {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()

	ts.ntpAvailable = true
	ts.ntpValue = value
	ts.ntpTimestamp = timestamp
}

func (ts *clientTimeConvMPEGTS) setLeadingNTPReceived() {
	select {
	case <-ts.chLeadingNTPReceived:
		return
	default:
	}
	close(ts.chLeadingNTPReceived)
}

func (ts *clientTimeConvMPEGTS) getNTP(ctx context.Context, timestamp int64) *time.Time {
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

	v := ts.ntpValue.Add(timestampToDuration(timestamp-ts.ntpTimestamp, 90000))

	return &v
}
