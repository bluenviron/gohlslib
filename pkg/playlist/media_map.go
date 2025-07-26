package playlist

import (
	"fmt"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist/primitives"
)

// MediaMap is a EXT-X-MAP tag.
type MediaMap struct {
	// URI
	// required
	URI string

	// BYTERANGE
	ByteRangeLength *uint64
	ByteRangeStart  *uint64
}

func (t *MediaMap) unmarshal(v string) error {
	var attrs primitives.Attributes
	err := attrs.Unmarshal(v)
	if err != nil {
		return err
	}

	for key, val := range attrs {
		switch key {
		case "URI":
			t.URI = val

		case "BYTERANGE":
			var br primitives.ByteRange
			err = br.Unmarshal(val)
			if err != nil {
				return err
			}

			t.ByteRangeLength = &br.Length
			t.ByteRangeStart = br.Start
		}
	}

	if t.URI == "" {
		return fmt.Errorf("URI not found")
	}

	return nil
}

func (t MediaMap) marshal() string {
	ret := "#EXT-X-MAP:URI=\"" + t.URI + "\""

	if t.ByteRangeLength != nil {
		ret += ",BYTERANGE=" + primitives.ByteRange{
			Length: *t.ByteRangeLength,
			Start:  t.ByteRangeStart,
		}.Marshal() + ""
	}

	ret += "\n"

	return ret
}
