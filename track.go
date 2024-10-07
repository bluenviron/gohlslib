package gohlslib

import (
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

// Track is a HLS track.
type Track struct {
	Codec     codecs.Codec
	ClockRate int
}
