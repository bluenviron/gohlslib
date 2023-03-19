package playlist

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMultivariantMarshal(t *testing.T) {
	p := &Multivariant{
		Version:             9,
		IndependentSegments: true,
		Variants: []*MultivariantVariant{{
			Bandwidth: 155000,
			Codecs: []string{
				"avc1.42c028",
				"mp4a.40.2",
			},
			Resolution: func() *string {
				v := "1280x720"
				return &v
			}(),
			FrameRate: func() *float64 {
				v := 24.0
				return &v
			}(),
			URL: "stream.m3u8",
		}},
	}

	byts, err := p.Marshal()
	require.NoError(t, err)
	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:9\n"+
		"#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=155000,CODECS=\"avc1.42c028,mp4a.40.2\",RESOLUTION=1280x720,FRAME-RATE=24.000\n"+
		"stream.m3u8\n",
		string(byts))
}
