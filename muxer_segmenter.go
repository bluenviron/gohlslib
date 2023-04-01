package gohlslib

import (
	"time"
)

type muxerSegmenter interface {
	close()
	writeH26x(time.Time, time.Duration, [][]byte, bool, bool) error
	writeAudio(time.Time, time.Duration, []byte) error
}
