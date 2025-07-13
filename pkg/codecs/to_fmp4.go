package codecs

import "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4"

// ToFMP4 converts a codec in its fMP4 equivalent.
func ToFMP4(in Codec) mp4.Codec {
	switch in := in.(type) {
	case *AV1:
		return &mp4.CodecAV1{
			SequenceHeader: in.SequenceHeader,
		}

	case *VP9:
		return &mp4.CodecVP9{
			Width:             in.Width,
			Height:            in.Height,
			Profile:           in.Profile,
			BitDepth:          in.BitDepth,
			ChromaSubsampling: in.ChromaSubsampling,
			ColorRange:        in.ColorRange,
		}

	case *H265:
		return &mp4.CodecH265{
			VPS: in.VPS,
			SPS: in.SPS,
			PPS: in.PPS,
		}

	case *H264:
		return &mp4.CodecH264{
			SPS: in.SPS,
			PPS: in.PPS,
		}

	case *Opus:
		return &mp4.CodecOpus{
			ChannelCount: in.ChannelCount,
		}

	case *MPEG4Audio:
		return &mp4.CodecMPEG4Audio{
			Config: in.Config,
		}
	}

	return nil
}
