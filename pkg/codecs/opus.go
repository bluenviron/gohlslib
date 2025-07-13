package codecs

// Opus is a Opus codec.
type Opus struct {
	ChannelCount int
}

// IsVideo returns whether the codec is a video one.
func (*Opus) IsVideo() bool {
	return false
}

func (*Opus) isCodec() {
}
