package codecs

// MPEG1Audio represents MPEG-1/2 Layer II/III audio (commonly MP3) for MPEG-TS HLS.
// It carries raw MPEG audio frames and is only supported with the MPEG-TS variant.
type MPEG1Audio struct{}

// IsVideo implements Codec.
func (MPEG1Audio) IsVideo() bool { return false }

func (*MPEG1Audio) isCodec() {}
