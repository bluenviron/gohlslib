package codecs

import (
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
)

// FromMPEGTS imports a codec from MPEG-TS.
func FromMPEGTS(in codecs.Codec) Codec {
	switch in := in.(type) {
	case *codecs.H264:
		return &H264{}

	case *codecs.MPEG4Audio:
		return &MPEG4Audio{
			Config: in.Config,
		}

	case *codecs.KLV:
		return &KLV{
			Synchronous: in.Synchronous,
		}
	}

	return nil
}
