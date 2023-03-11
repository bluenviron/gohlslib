package playlist

import (
	"strconv"
	"time"
)

// MediaServerControl is a EXT-X-SERVER-CONTROL tag.
type MediaServerControl struct {
	CanBlockReload bool

	// The value is a decimal-floating-point number of seconds that
	// indicates the server-recommended minimum distance from the end of
	// the Playlist at which clients should begin to play or to which
	// they should seek when playing in Low-Latency Mode.  Its value MUST
	// be at least twice the Part Target Duration.  Its value SHOULD be
	// at least three times the Part Target Duration.
	PartHoldBack *time.Duration

	// Indicates that the Server can produce Playlist Delta Updates in
	// response to the _HLS_skip Delivery Directive.  Its value is the
	// Skip Boundary, a decimal-floating-point number of seconds.  The
	// Skip Boundary MUST be at least six times the Target Duration.
	CanSkipUntil *time.Duration
}

func (t MediaServerControl) marshal() string {
	ret := "#EXT-X-SERVER-CONTROL:"

	if t.CanBlockReload {
		ret += "CAN-BLOCK-RELOAD=YES"
	}

	if t.PartHoldBack != nil {
		ret += ",PART-HOLD-BACK=" + strconv.FormatFloat(t.PartHoldBack.Seconds(), 'f', 5, 64)
	}

	if t.CanSkipUntil != nil {
		ret += ",CAN-SKIP-UNTIL=" + strconv.FormatFloat(t.CanSkipUntil.Seconds(), 'f', -1, 64)
	}

	ret += "\n"
	return ret
}

// MediaPartInf is a EXT-X-PART-INF tag.
type MediaPartInf struct {
	PartTarget time.Duration
}

func (t MediaPartInf) marshal() string {
	return "#EXT-X-PART-INF:PART-TARGET=" + strconv.FormatFloat(t.PartTarget.Seconds(), 'f', -1, 64) + "\n"
}

// MediaMap is a EXT-X-MAP tag.
type MediaMap struct {
	URI string
}

func (t MediaMap) marshal() string {
	return "#EXT-X-MAP:URI=\"" + t.URI + "\"\n"
}

// MediaSkip is a EXT-X-SKIP tag.
type MediaSkip struct {
	SkippedSegments int
}

func (t MediaSkip) marshal() string {
	return "#EXT-X-SKIP:SKIPPED-SEGMENTS=" + strconv.FormatInt(int64(t.SkippedSegments), 10) + "\n"
}

// MediaPart is a EXT-X-PART tag.
type MediaPart struct {
	Duration    time.Duration
	Independent bool
	URI         string
}

func (p MediaPart) marshal() string {
	ret := "#EXT-X-PART:DURATION=" + strconv.FormatFloat(p.Duration.Seconds(), 'f', 5, 64) +
		",URI=\"" + p.URI + "\""

	if p.Independent {
		ret += ",INDEPENDENT=YES"
	}

	ret += "\n"
	return ret
}

// MediaSegment is a segment of a media playlist.
type MediaSegment struct {
	DateTime *time.Time
	Gap      bool
	Duration time.Duration
	URI      string
	Parts    []*MediaPart
}

func (s MediaSegment) marshal() string {
	ret := ""

	if s.DateTime != nil {
		ret += "#EXT-X-PROGRAM-DATE-TIME:" + s.DateTime.Format("2006-01-02T15:04:05.999Z07:00") + "\n"
	}

	if s.Gap {
		ret += "#EXT-X-GAP\n"
	}

	for _, part := range s.Parts {
		ret += part.marshal()
	}

	ret += "#EXTINF:" + strconv.FormatFloat(s.Duration.Seconds(), 'f', 5, 64) + ",\n" +
		s.URI + "\n"

	return ret
}

// MediaPreloadHint sia EXT-X-PRELOAD-HINT tag.
type MediaPreloadHint struct {
	URI string
}

func (t MediaPreloadHint) marshal() string {
	return "#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"" + t.URI + "\"\n"
}

// Media is a media playlist.
type Media struct {
	Version        int
	AllowCache     *bool // removed in v7
	TargetDuration int
	ServerControl  *MediaServerControl
	PartInf        *MediaPartInf
	MediaSequence  int
	Map            *MediaMap
	Skip           *MediaSkip
	Segments       []*MediaSegment
	Parts          []*MediaPart
	PreloadHint    *MediaPreloadHint
}

// Marshal encodes the playlist.
func (m Media) Marshal() ([]byte, error) {
	ret := "#EXTM3U\n" +
		"#EXT-X-VERSION:" + strconv.FormatInt(int64(m.Version), 10) + "\n"

	if m.AllowCache != nil {
		var v string
		if *m.AllowCache {
			v = "YES"
		} else {
			v = "NO"
		}
		ret += "#EXT-X-ALLOW-CACHE:" + v + "\n"
	}

	ret += "#EXT-X-TARGETDURATION:" + strconv.FormatInt(int64(m.TargetDuration), 10) + "\n"

	if m.ServerControl != nil {
		ret += m.ServerControl.marshal()
	}

	if m.PartInf != nil {
		ret += m.PartInf.marshal()
	}

	ret += "#EXT-X-MEDIA-SEQUENCE:" + strconv.FormatInt(int64(m.MediaSequence), 10) + "\n"

	if m.Map != nil {
		ret += m.Map.marshal()
	}

	if m.Skip != nil {
		ret += m.Skip.marshal()
	}

	for _, seg := range m.Segments {
		ret += seg.marshal()
	}

	for _, part := range m.Parts {
		ret += part.marshal()
	}

	if m.PreloadHint != nil {
		ret += m.PreloadHint.marshal()
	}

	return []byte(ret), nil
}
