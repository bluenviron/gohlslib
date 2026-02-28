package gohlslib

import (
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
)

func fromMPEGTS(in tscodecs.Codec) codecs.Codec {
	switch in := in.(type) {
	case *tscodecs.H264:
		return &codecs.H264{}

	case *tscodecs.MPEG4Audio:
		return &codecs.MPEG4Audio{
			Config: in.Config,
		}
	}

	return nil
}

func toMPEGTS(in codecs.Codec) tscodecs.Codec {
	switch in := in.(type) {
	case *codecs.H264:
		return &tscodecs.H264{}

	case *codecs.MPEG4Audio:
		return &tscodecs.MPEG4Audio{
			Config: in.Config,
		}
	}

	return nil
}
