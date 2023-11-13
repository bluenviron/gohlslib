package codecparams

import (
	"testing"

	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/stretchr/testify/require"

	"github.com/vicon-security/gohlslib/pkg/codecs"
)

func TestMarshal(t *testing.T) {
	for _, ca := range []struct {
		name  string
		codec codecs.Codec
		enc   string
	}{
		{
			"av1",
			&codecs.AV1{
				SequenceHeader: []byte{
					10, 11, 0, 0, 0, 66, 167, 191, 230, 46, 223, 200, 66,
				},
			},
			"av01.0.08M.08.0.110.01.01.01.0",
		},
		{
			"vp9",
			&codecs.VP9{
				Width:             1920,
				Height:            1080,
				Profile:           1,
				BitDepth:          8,
				ChromaSubsampling: 1,
				ColorRange:        false,
			},
			"vp09.01.10.08",
		},
		{
			"h265",
			&codecs.H265{
				VPS: []byte{0x01, 0x02, 0x03, 0x04},
				SPS: []byte{
					0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x03,
					0x00, 0x90, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03,
					0x00, 0x78, 0xa0, 0x03, 0xc0, 0x80, 0x10, 0xe5,
					0x96, 0x66, 0x69, 0x24, 0xca, 0xe0, 0x10, 0x00,
					0x00, 0x03, 0x00, 0x10, 0x00, 0x00, 0x03, 0x01,
					0xe0, 0x80,
				},
				PPS: []byte{0x08},
			},
			"hvc1.1.6.L120.90",
		},
		{
			"h264",
			&codecs.H264{
				SPS: []byte{
					0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
					0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
					0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
					0x20,
				},
				PPS: []byte{0x08},
			},
			"avc1.42c028",
		},
		{
			"opus",
			&codecs.Opus{},
			"opus",
		},
		{
			"mpeg-4 audio",
			&codecs.MPEG4Audio{
				Config: mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
			},
			"mp4a.40.2",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			enc := Marshal(ca.codec)
			require.Equal(t, ca.enc, enc)
		})
	}
}
