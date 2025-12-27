package codecs

import "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"

// FromFMP4 imports a codec from fMP4.
func FromFMP4(in codecs.Codec) Codec {
	switch in := in.(type) {
	case *codecs.AV1:
		return &AV1{
			SequenceHeader: in.SequenceHeader,
		}

	case *codecs.VP9:
		return &VP9{
			Width:             in.Width,
			Height:            in.Height,
			Profile:           in.Profile,
			BitDepth:          in.BitDepth,
			ChromaSubsampling: in.ChromaSubsampling,
			ColorRange:        in.ColorRange,
		}

	case *codecs.H265:
		return &H265{
			VPS: in.VPS,
			SPS: in.SPS,
			PPS: in.PPS,
		}

	case *codecs.H264:
		return &H264{
			SPS: in.SPS,
			PPS: in.PPS,
		}

	case *codecs.Opus:
		return &Opus{
			ChannelCount: in.ChannelCount,
		}

	case *codecs.MPEG4Audio:
		return &MPEG4Audio{
			Config: in.Config,
		}
	}

	return nil
}
