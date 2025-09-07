package playlist

import (
	"fmt"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist/primitives"
)

// MediaKeyMethod is the encryption method used for the media segments.
type MediaKeyMethod string

// standard encryption methods
const (
	MediaKeyMethodNone      MediaKeyMethod = "NONE"
	MediaKeyMethodAES128    MediaKeyMethod = "AES-128"
	MediaKeyMethodSampleAES MediaKeyMethod = "SAMPLE-AES"
)

// MediaKey is a EXT-X-KEY tag.
type MediaKey struct {
	// METHOD
	// required
	Method MediaKeyMethod

	// URI is required unless METHOD is NONE
	URI string

	// IV
	IV string

	// KEYFORMAT
	KeyFormat string

	// KEYFORMATVERSIONS
	KeyFormatVersions string
}

func (t *MediaKey) unmarshal(v string) error {
	var attrs primitives.Attributes
	err := attrs.Unmarshal(v)
	if err != nil {
		return err
	}

	for key, val := range attrs {
		switch key {
		case "METHOD":
			km := MediaKeyMethod(val)
			if km != MediaKeyMethodNone &&
				km != MediaKeyMethodAES128 &&
				km != MediaKeyMethodSampleAES {
				return fmt.Errorf("invalid method: %s", val)
			}
			t.Method = km

		case "URI":
			t.URI = val

		case "IV":
			t.IV = val

		case "KEYFORMAT":
			t.KeyFormat = val

		case "KEYFORMATVERSIONS":
			t.KeyFormatVersions = val
		}
	}

	switch t.Method {
	case MediaKeyMethodAES128, MediaKeyMethodSampleAES:
		if t.URI == "" {
			return fmt.Errorf("URI is required for method %s", t.Method)
		}
	default:
	}

	return nil
}

func (t MediaKey) marshal() string {
	ret := "#EXT-X-KEY:METHOD=" + string(t.Method)

	// If the encryption method is NONE, other attributes MUST NOT be present.
	if t.Method != MediaKeyMethodNone {
		ret += ",URI=\"" + t.URI + "\""

		if t.IV != "" {
			ret += ",IV=" + t.IV
		}

		if t.KeyFormat != "" {
			ret += ",KEYFORMAT=\"" + t.KeyFormat + "\""
		}

		if t.KeyFormatVersions != "" {
			ret += ",KEYFORMATVERSIONS=\"" + t.KeyFormatVersions + "\""
		}
	}

	ret += "\n"

	return ret
}

// Equal checks if two MediaKey objects are equal.
func (t *MediaKey) Equal(key *MediaKey) bool {
	if t == key {
		return true
	}

	if key == nil {
		return false
	}

	return t.Method == key.Method &&
		t.URI == key.URI &&
		t.IV == key.IV &&
		t.KeyFormat == key.KeyFormat &&
		t.KeyFormatVersions == key.KeyFormatVersions
}
