package codecs

import (
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
)

// MPEG4Audio is a MPEG-4 Audio codec.
type MPEG4Audio struct {
	mpeg4audio.Config
}

func (*MPEG4Audio) isCodec() {
}
