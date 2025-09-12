package playlist

import (
	"fmt"
	"strconv"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist/primitives"
)

// MediaRenditionReport is a EXT-X-RENDITION-REPORT tag.
type MediaRenditionReport struct {
	// URI (required)
	URI string

	// LAST-MSN (required)
	LastMSN int

	// LAST-PART
	LastPart *int
}

func (r *MediaRenditionReport) unmarshal(v string) error {
	var attrs primitives.Attributes
	err := attrs.Unmarshal(v)
	if err != nil {
		return err
	}

	lastMsnRecv := false
	for key, val := range attrs {
		switch key {
		case "URI":
			r.URI = val

		case "LAST-MSN":
			var tmp uint64
			tmp, err = strconv.ParseUint(val, 10, 31)
			if err != nil {
				return err
			}
			r.LastMSN = int(tmp)
			lastMsnRecv = true

		case "LAST-PART":
			var tmp uint64
			tmp, err = strconv.ParseUint(val, 10, 31)
			if err != nil {
				return err
			}
			value := int(tmp)
			r.LastPart = &value
		}
	}

	if r.URI == "" {
		return fmt.Errorf("URI is missing")
	}

	if !lastMsnRecv {
		return fmt.Errorf("LAST-MSN is missing")
	}

	return nil
}

func (r MediaRenditionReport) marshal() string {
	ret := "#EXT-X-RENDITION-REPORT:URI=\"" + r.URI + "\""

	ret += ",LAST-MSN=" + strconv.FormatInt(int64(r.LastMSN), 10)

	if r.LastPart != nil {
		ret += ",LAST-PART=" + strconv.FormatInt(int64(*r.LastPart), 10)
	}

	ret += "\n"

	return ret
}
