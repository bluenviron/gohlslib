package playlist

import (
	"fmt"
	"strconv"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist/primitives"
)

// MediaPreloadHint sia EXT-X-PRELOAD-HINT tag.
type MediaPreloadHint struct {
	// URI
	// required
	URI string

	// BYTERANGE-START
	ByteRangeStart uint64

	// BYTERANGE-LENGTH
	ByteRangeLength *uint64
}

func (t *MediaPreloadHint) unmarshal(v string) error {
	var attrs primitives.Attributes
	err := attrs.Unmarshal(v)
	if err != nil {
		return err
	}

	typeRecv := false

	for key, val := range attrs {
		switch key {
		case "TYPE":
			if val != "PART" {
				return fmt.Errorf("unsupported type: %s", val)
			}
			typeRecv = true

		case "URI":
			t.URI = val

		case "BYTERANGE-START":
			var tmp uint64
			tmp, err = strconv.ParseUint(val, 10, 64)
			if err != nil {
				return err
			}
			t.ByteRangeStart = tmp

		case "BYTERANGE-LENGTH":
			var tmp uint64
			tmp, err = strconv.ParseUint(val, 10, 64)
			if err != nil {
				return err
			}
			t.ByteRangeLength = &tmp
		}
	}

	if !typeRecv {
		return fmt.Errorf("TYPE is missing")
	}

	if t.URI == "" {
		return fmt.Errorf("URI is missing")
	}

	return nil
}

func (t MediaPreloadHint) marshal() string {
	ret := "#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"" + t.URI + "\""

	if t.ByteRangeStart != 0 {
		ret += ",BYTERANGE-START=" + strconv.FormatUint(t.ByteRangeStart, 10)
	}

	if t.ByteRangeLength != nil {
		ret += ",BYTERANGE-LENGTH=" + strconv.FormatUint(*t.ByteRangeLength, 10)
	}

	ret += "\n"

	return ret
}
