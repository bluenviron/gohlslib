package codecs

import "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"

// ToFMP4 converts a codec in its fMP4 equivalent.
func ToFMP4(in Codec) codecs.Codec {
	switch in := in.(type) {
	case *AV1:
		return &codecs.AV1{
			SequenceHeader: in.SequenceHeader,
		}

	case *VP9:
		return &codecs.VP9{
			Width:             in.Width,
			Height:            in.Height,
			Profile:           in.Profile,
			BitDepth:          in.BitDepth,
			ChromaSubsampling: in.ChromaSubsampling,
			ColorRange:        in.ColorRange,
		}

	case *H265:
		return &codecs.H265{
			VPS: in.VPS,
			SPS: in.SPS,
			PPS: in.PPS,
		}

	case *H264:
		return &codecs.H264{
			SPS: in.SPS,
			PPS: in.PPS,
		}

	case *Opus:
		return &codecs.Opus{
			ChannelCount: in.ChannelCount,
		}

	case *MPEG4Audio:
		return &codecs.MPEG4Audio{
			Config: in.Config,
		}
	}

	return nil
}
