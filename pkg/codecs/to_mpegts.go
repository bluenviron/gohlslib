package codecs

import "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"

// ToMPEGTS converts a codec in its MPEG-TS equivalent.
func ToMPEGTS(in Codec) mpegts.Codec {
	switch in := in.(type) {
	case *H264:
		return &mpegts.CodecH264{}

	case *MPEG4Audio:
		return &mpegts.CodecMPEG4Audio{
			Config: in.Config,
		}
	}

	return nil
}
