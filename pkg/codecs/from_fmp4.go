package codecs

import "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4"

// FromFMP4 imports a codec from fMP4.
func FromFMP4(in mp4.Codec) Codec {
	switch in := in.(type) {
	case *mp4.CodecAV1:
		return &AV1{
			SequenceHeader: in.SequenceHeader,
		}

	case *mp4.CodecVP9:
		return &VP9{
			Width:             in.Width,
			Height:            in.Height,
			Profile:           in.Profile,
			BitDepth:          in.BitDepth,
			ChromaSubsampling: in.ChromaSubsampling,
			ColorRange:        in.ColorRange,
		}

	case *mp4.CodecH265:
		return &H265{
			VPS: in.VPS,
			SPS: in.SPS,
			PPS: in.PPS,
		}

	case *mp4.CodecH264:
		return &H264{
			SPS: in.SPS,
			PPS: in.PPS,
		}

	case *mp4.CodecOpus:
		return &Opus{
			ChannelCount: in.ChannelCount,
		}

	case *mp4.CodecMPEG4Audio:
		return &MPEG4Audio{
			Config: in.Config,
		}
	}

	return nil
}
