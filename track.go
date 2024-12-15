package gohlslib

import (
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

// Track is a HLS track.
type Track struct {
	// Codec
	Codec codecs.Codec

	// Clock rate
	ClockRate int

	// Name
	// For audio renditions only.
	Name string

	// Language
	// For audio renditions only.
	Language string

	// whether this is the default track.
	// For audio renditions only.
	IsDefault bool
}
