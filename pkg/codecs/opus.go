package codecs

// Opus is a Opus codec.
type Opus struct {
	Channels int
}

func (*Opus) isCodec() {}
