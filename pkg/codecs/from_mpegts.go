package codecs

import (
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
)

// FromMPEGTS imports a codec from MPEG-TS.
func FromMPEGTS(in mpegts.Codec) Codec {
	switch in := in.(type) {
	case *mpegts.CodecH264:
		return &H264{}

	case *mpegts.CodecMPEG4Audio:
		return &MPEG4Audio{
			Config: in.Config,
		}
	}

	return nil
}
