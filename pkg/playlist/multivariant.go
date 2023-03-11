package playlist

import (
	"strconv"
	"strings"
)

// MultivariantVariant is a variant of a multivariant playlist.
type MultivariantVariant struct {
	Bandwidth int
	Codecs    []string
	URL       string
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
		ret += "#EXT-X-STREAM-INF:BANDWIDTH=" + strconv.FormatInt(int64(v.Bandwidth), 10) +
			",CODECS=\"" + strings.Join(v.Codecs, ",") + "\"\n" +
			v.URL + "\n"
	}

	return []byte(ret), nil
}
