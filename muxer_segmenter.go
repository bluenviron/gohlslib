package gohlslib

import (
	"time"
)

type muxerSegmenter interface {
	close()
	writeAV1(time.Time, time.Duration, [][]byte, bool, bool) error
	writeH26x(time.Time, time.Duration, [][]byte, bool, bool) error
	writeOpus(time.Time, time.Duration, [][]byte) error
	writeMPEG4Audio(time.Time, time.Duration, [][]byte) error
}
