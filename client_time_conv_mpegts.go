package gohlslib

import (
	"sync"

	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
)

type clientTimeConvMPEGTS struct {
	startDTS int64

	td    *mpegts.TimeDecoder2
	mutex sync.Mutex
}

func (ts *clientTimeConvMPEGTS) initialize() {
	ts.td = mpegts.NewTimeDecoder2(ts.startDTS)
}

func (ts *clientTimeConvMPEGTS) convert(v int64) int64 {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()
	return ts.td.Decode(v)
}
