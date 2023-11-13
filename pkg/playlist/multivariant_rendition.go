package playlist

import (
	"fmt"

	"github.com/vicon-security/gohlslib/pkg/playlist/primitives"
)

// MultivariantRenditionType is a rendition type.
type MultivariantRenditionType string

// standard rendition types.
const (
	MultivariantRenditionTypeAudio          = "AUDIO"
	MultivariantRenditionTypeVideo          = "VIDEO"
	MultivariantRenditionTypeSubtitles      = "SUBTITLES"
	MultivariantRenditionTypeClosedCaptions = "CLOSED-CAPTIONS"
)

// MultivariantRendition is a EXT-X-MEDIA tag.
type MultivariantRendition struct {
	// TYPE
	// required
	Type MultivariantRenditionType

	// GROUP-ID
	// required
	GroupID string

	// URI
	// required for all types except CLOSED-CAPTIONS
	URI string

	// INSTREAM-ID
	// required for CLOSED-CAPTIONS
	InstreamID string

	// NAME
	// required
	Name string

	// LANGUAGE
	Language string

	// DEFAULT
	Default bool

	// AUTOSELECT
	Autoselect bool

	// FORCED
	Forced *bool

	// CHANNELS
	Channels string
}

func (t *MultivariantRendition) unmarshal(v string) error {
	attrs, err := primitives.AttributesUnmarshal(v)
	if err != nil {
		return err
	}

	for key, val := range attrs {
		switch key {
		case "TYPE":
			rt := MultivariantRenditionType(val)
			if rt != MultivariantRenditionTypeAudio &&
				rt != MultivariantRenditionTypeVideo &&
				rt != MultivariantRenditionTypeSubtitles &&
				rt != MultivariantRenditionTypeClosedCaptions {
				return fmt.Errorf("invalid type: %s", val)
			}
			t.Type = rt

		case "GROUP-ID":
			t.GroupID = val

		case "LANGUAGE":
			t.Language = val

		case "NAME":
			t.Name = val

		case "DEFAULT":
			t.Default = (val == "YES")

		case "AUTOSELECT":
			t.Autoselect = (val == "YES")

		case "FORCED":
			v := (val == "YES")
			t.Forced = &v

		case "CHANNELS":
			t.Channels = val

		case "URI":
			t.URI = val

		case "INSTREAM-ID":
			t.InstreamID = val
		}
	}

	if t.Type == "" {
		return fmt.Errorf("missing type")
	}

	if t.GroupID == "" {
		return fmt.Errorf("GROUP-ID missing")
	}

	if t.Type != MultivariantRenditionTypeClosedCaptions {
		if t.URI == "" {
			return fmt.Errorf("missing URI")
		}

		if t.InstreamID != "" {
			return fmt.Errorf("INSTREAM-ID is forbidden with type %s", t.Type)
		}
	} else {
		if t.URI != "" {
			return fmt.Errorf("URI is forbidden for type CLOSED-CAPTIONS")
		}

		if t.InstreamID == "" {
			return fmt.Errorf("missing INSTREAM-ID")
		}
	}

	return nil
}

func (t MultivariantRendition) marshal() string {
	ret := "#EXT-X-MEDIA:TYPE=\"" + string(t.Type) + "\",GROUP-ID=\"" + t.GroupID + "\""

	if t.Language != "" {
		ret += ",LANGUAGE=\"" + t.Language + "\""
	}

	if t.Name != "" {
		ret += ",NAME=\"" + t.Name + "\""
	}

	if t.Default {
		ret += ",DEFAULT=YES"
	}

	if t.Autoselect {
		ret += ",AUTOSELECT=YES"
	}

	if t.Forced != nil {
		ret += ",FORCED="
		if *t.Forced {
			ret += "YES"
		} else {
			ret += "NO"
		}
	}

	if t.Channels != "" {
		ret += ",CHANNELS=\"" + t.Channels + "\""
	}

	ret += ",URI=\"" + t.URI + "\"\n"

	return ret
}
