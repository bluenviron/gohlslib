package codecs

import "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"

// ToMPEGTS converts a codec in its MPEG-TS equivalent.
func ToMPEGTS(in Codec) mpegts.Codec {
	switch in := in.(type) {
	case *H264:
		return &mpegts.CodecH264{}

	case *H265:
		return &mpegts.CodecH265{}

	case *MPEG4Audio:
		return &mpegts.CodecMPEG4Audio{
			Config: in.Config,
		}

	case *MPEG1Audio:
		return &mpegts.CodecMPEG1Audio{}
	}

	return nil
}
