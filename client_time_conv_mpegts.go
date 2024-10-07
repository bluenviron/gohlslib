package gohlslib

import (
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
)

type clientTimeConvMPEGTS struct {
	startDTS int64

	mutex        sync.Mutex
	td           *mpegts.TimeDecoder2
	ntpAvailable bool
	ntpValue     time.Time
	ntpTimestamp int64
}

func (ts *clientTimeConvMPEGTS) initialize() {
	ts.td = mpegts.NewTimeDecoder2()
	ts.td.Decode(ts.startDTS)
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

func (ts *clientTimeConvMPEGTS) getNTP(timestamp int64) *time.Time {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()

	if !ts.ntpAvailable {
		return nil
	}

	v := ts.ntpValue.Add(timestampToDuration(timestamp-ts.ntpTimestamp, 90000))
	return &v
}
