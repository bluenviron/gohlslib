package codecs

import (
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
)

// ToMPEGTS converts a codec in its MPEG-TS equivalent.
func ToMPEGTS(in Codec) codecs.Codec {
	switch in := in.(type) {
	case *H264:
		return &codecs.H264{}

	case *MPEG4Audio:
		return &codecs.MPEG4Audio{
			Config: in.Config,
		}
	}

	return nil
}
