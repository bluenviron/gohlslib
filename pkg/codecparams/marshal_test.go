package codecparams

import (
	"testing"

	"github.com/aler9/gortsplib/v2/pkg/codecs/mpeg4audio"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/stretchr/testify/require"
)

func TestMarshal(t *testing.T) {
	t.Run("h264", func(t *testing.T) {
		p := Marshal(&format.H264{
			PayloadTyp: 96,
			SPS: []byte{
				0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
				0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
				0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
				0x20,
			},
			PPS:               []byte{0x08},
			PacketizationMode: 1,
		})
		require.Equal(t, "avc1.42c028", p)
	})

	t.Run("h265", func(t *testing.T) {
		p := Marshal(&format.H265{
			PayloadTyp: 96,
			VPS:        []byte{0x01, 0x02, 0x03, 0x04},
			SPS: []byte{
				0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x03,
				0x00, 0x90, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03,
				0x00, 0x78, 0xa0, 0x03, 0xc0, 0x80, 0x10, 0xe5,
				0x96, 0x66, 0x69, 0x24, 0xca, 0xe0, 0x10, 0x00,
				0x00, 0x03, 0x00, 0x10, 0x00, 0x00, 0x03, 0x01,
				0xe0, 0x80,
			},
			PPS: []byte{0x08},
		})
		require.Equal(t, "hvc1.1.6.L120.90", p)
	})

	t.Run("mpeg4-audio", func(t *testing.T) {
		p := Marshal(&format.MPEG4Audio{
			PayloadTyp: 97,
			Config: &mpeg4audio.Config{
				Type:         2,
				SampleRate:   44100,
				ChannelCount: 2,
			},
			SizeLength:       13,
			IndexLength:      3,
			IndexDeltaLength: 3,
		})
		require.Equal(t, "mp4a.40.2", p)
	})

	t.Run("opus", func(t *testing.T) {
		p := Marshal(&format.Opus{})
		require.Equal(t, "opus", p)
	})
}
