package playlist

import (
	"fmt"
	"strconv"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist/primitives"
)

// MediaPart is a EXT-X-PART tag.
type MediaPart struct {
	// DURATION
	// required
	Duration time.Duration

	// URI
	// required
	URI string

	// INDEPENDENT
	Independent bool

	// BYTERANGE
	ByteRangeLength *uint64
	ByteRangeStart  *uint64

	// GAP
	Gap bool
}

func (p *MediaPart) unmarshal(v string) error {
	var attrs primitives.Attributes
	err := attrs.Unmarshal(v)
	if err != nil {
		return err
	}

	for key, val := range attrs {
		switch key {
		case "DURATION":
			var d primitives.Duration
			err = d.Unmarshal(val)
			if err != nil {
				return err
			}
			p.Duration = time.Duration(d)

		case "URI":
			p.URI = val

		case "INDEPENDENT":
			p.Independent = (val == "YES")

		case "BYTERANGE":
			var br primitives.ByteRange
			err = br.Unmarshal(val)
			if err != nil {
				return err
			}
			p.ByteRangeLength = &br.Length
			p.ByteRangeStart = br.Start

		case "GAP":
			p.Gap = true
		}
	}

	if p.Duration == 0 {
		return fmt.Errorf("DURATION missing")
	}

	if p.URI == "" {
		return fmt.Errorf("URI missing")
	}

	return nil
}

func (p MediaPart) marshal() string {
	ret := "#EXT-X-PART:DURATION=" + strconv.FormatFloat(p.Duration.Seconds(), 'f', 5, 64) +
		",URI=\"" + p.URI + "\""

	if p.Independent {
		ret += ",INDEPENDENT=YES"
	}

	if p.ByteRangeLength != nil {
		ret += ",BYTERANGE=" + primitives.ByteRange{
			Length: *p.ByteRangeLength,
			Start:  p.ByteRangeStart,
		}.Marshal() + ""
	}

	if p.Gap {
		ret += ",GAP=YES"
	}

	ret += "\n"
	return ret
}
