package gohlslib

import (
	"time"
)

type muxerSegmenter interface {
	close()
	writeH26x(time.Time, time.Duration, [][]byte, bool, bool) error
	writeMPEG4Audio(time.Time, time.Duration, [][]byte) error
	writeOpus(time.Time, time.Duration, [][]byte) error
}
