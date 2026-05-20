package codecs

import "github.com/bluenviron/mediacommon/v2/pkg/codecs/flac"

// FLAC is a FLAC codec.
type FLAC struct {
	StreamInfo *flac.StreamInfo
}

// IsVideo returns whether the codec is a video one.
func (*FLAC) IsVideo() bool {
	return false
}

func (*FLAC) isCodec() {
}
