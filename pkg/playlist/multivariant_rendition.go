package playlist

import (
	"fmt"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist/primitives"
)

// MultivariantRenditionType is a rendition type.
type MultivariantRenditionType string

// standard rendition types.
const (
	MultivariantRenditionTypeAudio          MultivariantRenditionType = "AUDIO"
	MultivariantRenditionTypeVideo          MultivariantRenditionType = "VIDEO"
	MultivariantRenditionTypeSubtitles      MultivariantRenditionType = "SUBTITLES"
	MultivariantRenditionTypeClosedCaptions MultivariantRenditionType = "CLOSED-CAPTIONS"
)

// MultivariantRendition is a EXT-X-MEDIA tag.
type MultivariantRendition struct {
	// TYPE
	// required
	Type MultivariantRenditionType

	// GROUP-ID
	// required
	GroupID string

	// NAME
	// required
	Name string

	// LANGUAGE
	Language string

	// AUTOSELECT
	Autoselect bool

	// DEFAULT
	Default bool

	// FORCED
	Forced bool

	// CHANNELS
	// for AUDIO only
	Channels *string

	// URI
	// must not be present for CLOSED-CAPTIONS
	URI *string

	// INSTREAM-ID
	// for CLOSED-CAPTIONS only
	InStreamID *string
}

func (t *MultivariantRendition) unmarshal(v string) error {
	var attrs primitives.Attributes
	err := attrs.Unmarshal(v)
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
			t.Forced = (val == "YES")

		case "CHANNELS":
			cval := val
			t.Channels = &cval

		case "URI":
			cval := val
			t.URI = &cval

		case "INSTREAM-ID":
			cval := val
			t.InStreamID = &cval
		}
	}

	if t.Type == "" {
		return fmt.Errorf("missing type")
	}

	if t.GroupID == "" {
		return fmt.Errorf("GROUP-ID missing")
	}

	// If the TYPE is CLOSED-CAPTIONS, the URI
	// attribute MUST NOT be present.
	// The URI attribute of the EXT-X-MEDIA tag is REQUIRED if the media
	// type is SUBTITLES, but OPTIONAL if the media type is VIDEO or AUDIO.
	switch t.Type {
	case MultivariantRenditionTypeClosedCaptions:
		if t.URI != nil {
			return fmt.Errorf("URI is forbidden for type CLOSED-CAPTIONS")
		}

	case MultivariantRenditionTypeSubtitles:
		if t.URI == nil {
			return fmt.Errorf("URI is required for type SUBTITLES")
		}

	default:
	}

	// This attribute is REQUIRED if the TYPE attribute is CLOSED-CAPTIONS
	// For all other TYPE values, the INSTREAM-ID MUST NOT be specified.
	if t.Type == MultivariantRenditionTypeClosedCaptions {
		if t.InStreamID == nil {
			return fmt.Errorf("missing INSTREAM-ID")
		}
	} else if t.InStreamID != nil {
		return fmt.Errorf("INSTREAM-ID is forbidden for type %s", t.Type)
	}

	// The CHANNELS attribute MUST NOT be present unless the TYPE is
	// AUDIO.
	if t.Channels != nil && t.Type != MultivariantRenditionTypeAudio {
		return fmt.Errorf("CHANNELS is forbidden for type %s", t.Type)
	}

	return nil
}

func (t MultivariantRendition) marshal() string {
	ret := "#EXT-X-MEDIA:TYPE=" + string(t.Type) + ",GROUP-ID=\"" + t.GroupID + "\""

	if t.Language != "" {
		ret += ",LANGUAGE=\"" + t.Language + "\""
	}

	if t.Name != "" {
		ret += ",NAME=\"" + t.Name + "\""
	}

	if t.Autoselect {
		ret += ",AUTOSELECT=YES"
	}

	if t.Default {
		ret += ",DEFAULT=YES"
	}

	if t.Forced {
		ret += ",FORCED=YES"
	}

	if t.Channels != nil {
		ret += ",CHANNELS=\"" + *t.Channels + "\""
	}

	if t.URI != nil {
		ret += ",URI=\"" + *t.URI + "\""
	}

	if t.InStreamID != nil {
		ret += ",INSTREAM-ID=\"" + *t.InStreamID + "\""
	}

	ret += "\n"

	return ret
}
