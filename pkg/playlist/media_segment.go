package playlist

import (
	"fmt"
	"strconv"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist/primitives"
)

// MediaSegment is a segment of a media playlist.
type MediaSegment struct {
	// EXTINF
	// required
	Duration time.Duration
	Title    string

	// URI.
	// required
	URI string

	// EXT-X-DISCONTINUITY
	Discontinuity bool

	// EXT-X-GAP
	Gap bool

	// EXT-X-PROGRAM-DATE-TIME
	DateTime *time.Time

	// EXT-X-BITRATE
	Bitrate *int

	// EXT-X-KEY
	Key *MediaKey

	// EXT-X-BYTERANGE
	ByteRangeLength *uint64
	ByteRangeStart  *uint64

	// EXT-X-PART
	Parts []*MediaPart
}

func (s MediaSegment) validate() error {
	if s.Duration == 0 {
		return fmt.Errorf("duration is missing")
	}

	return nil
}

func (s MediaSegment) marshal() string {
	ret := ""

	if s.Discontinuity {
		ret += "#EXT-X-DISCONTINUITY\n"
	}

	if s.Gap {
		ret += "#EXT-X-GAP\n"
	}

	if s.DateTime != nil {
		ret += "#EXT-X-PROGRAM-DATE-TIME:" + s.DateTime.Format(timeRFC3339Millis) + "\n"
	}

	if s.Bitrate != nil {
		ret += "#EXT-X-BITRATE:" + strconv.FormatInt(int64(*s.Bitrate), 10) + "\n"
	}

	for _, part := range s.Parts {
		ret += part.marshal()
	}

	ret += "#EXTINF:" + strconv.FormatFloat(s.Duration.Seconds(), 'f', 5, 64) + "," + s.Title + "\n"

	if s.ByteRangeLength != nil {
		ret += "#EXT-X-BYTERANGE:" + primitives.ByteRange{
			Length: *s.ByteRangeLength,
			Start:  s.ByteRangeStart,
		}.Marshal() + "\n"
	}

	ret += s.URI + "\n"

	return ret
}
