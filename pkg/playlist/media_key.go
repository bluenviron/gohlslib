package playlist

import (
	"fmt"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist/primitives"
)

type MediaKeyMethod string

const (
	MediaKeyMethodNone      = "NONE"
	MediaKeyMethodAES128    = "AES-128"
	MediaKeyMethodSampleAes = "SAMPLE-AES"
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
	attrs, err := primitives.AttributesUnmarshal(v)
	if err != nil {
		return err
	}

	for key, val := range attrs {
		switch key {
		case "METHOD":
			km := MediaKeyMethod(val)
			if km != MediaKeyMethodNone &&
				km != MediaKeyMethodAES128 &&
				km != MediaKeyMethodSampleAes {
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
	case MediaKeyMethodAES128, MediaKeyMethodSampleAes:
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
