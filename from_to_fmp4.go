package gohlslib

import (
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	mp4codecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
)

func fromFMP4(in mp4codecs.Codec) codecs.Codec { //nolint:dupl
	switch in := in.(type) {
	case *mp4codecs.AV1:
		return &codecs.AV1{
			SequenceHeader: in.SequenceHeader,
		}

	case *mp4codecs.VP9:
		return &codecs.VP9{
			Width:             in.Width,
			Height:            in.Height,
			Profile:           in.Profile,
			BitDepth:          in.BitDepth,
			ChromaSubsampling: in.ChromaSubsampling,
			ColorRange:        in.ColorRange,
		}

	case *mp4codecs.H265:
		return &codecs.H265{
			VPS: in.VPS,
			SPS: in.SPS,
			PPS: in.PPS,
		}

	case *mp4codecs.H264:
		return &codecs.H264{
			SPS: in.SPS,
			PPS: in.PPS,
		}

	case *mp4codecs.Opus:
		return &codecs.Opus{
			ChannelCount: in.ChannelCount,
		}

	case *mp4codecs.MPEG4Audio:
		return &codecs.MPEG4Audio{
			Config: in.Config,
		}
	}

	return nil
}

func toFMP4(in codecs.Codec) mp4codecs.Codec { //nolint:dupl
	switch in := in.(type) {
	case *codecs.AV1:
		return &mp4codecs.AV1{
			SequenceHeader: in.SequenceHeader,
		}

	case *codecs.VP9:
		return &mp4codecs.VP9{
			Width:             in.Width,
			Height:            in.Height,
			Profile:           in.Profile,
			BitDepth:          in.BitDepth,
			ChromaSubsampling: in.ChromaSubsampling,
			ColorRange:        in.ColorRange,
		}

	case *codecs.H265:
		return &mp4codecs.H265{
			VPS: in.VPS,
			SPS: in.SPS,
			PPS: in.PPS,
		}

	case *codecs.H264:
		return &mp4codecs.H264{
			SPS: in.SPS,
			PPS: in.PPS,
		}

	case *codecs.Opus:
		return &mp4codecs.Opus{
			ChannelCount: in.ChannelCount,
		}

	case *codecs.MPEG4Audio:
		return &mp4codecs.MPEG4Audio{
			Config: in.Config,
		}
	}

	return nil
}
