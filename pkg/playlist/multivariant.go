package playlist

import (
	"strconv"
	"strings"
)

// MultivariantVariant is a variant of a multivariant playlist.
type MultivariantVariant struct {
	Bandwidth  int
	Codecs     []string
	Resolution *string
	FrameRate  *float64
	URL        string
}

func (v MultivariantVariant) marshal() string {
	ret := "#EXT-X-STREAM-INF:BANDWIDTH=" + strconv.FormatInt(int64(v.Bandwidth), 10) +
		",CODECS=\"" + strings.Join(v.Codecs, ",") + "\""

	if v.Resolution != nil {
		ret += ",RESOLUTION=" + *v.Resolution
	}

	if v.FrameRate != nil {
		ret += ",FRAME-RATE=" + strconv.FormatFloat(*v.FrameRate, 'f', 3, 64)
	}

	ret += "\n" + v.URL + "\n"

	return ret
}

// Multivariant is a multivariant playlist.
type Multivariant struct {
	Version             int
	IndependentSegments bool
	Variants            []*MultivariantVariant
}

// Marshal encodes the playlist.
func (m Multivariant) Marshal() ([]byte, error) {
	ret := "#EXTM3U\n" +
		"#EXT-X-VERSION:" + strconv.FormatInt(int64(m.Version), 10) + "\n"

	if m.IndependentSegments {
		ret += "#EXT-X-INDEPENDENT-SEGMENTS\n"
	}

	ret += "\n"

	for _, v := range m.Variants {
		ret += v.marshal()
	}

	return []byte(ret), nil
}
