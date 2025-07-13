package codecs

// AV1 is a AV1 codec.
type AV1 struct {
	SequenceHeader []byte
}

// IsVideo returns whether the codec is a video one.
func (*AV1) IsVideo() bool {
	return true
}

func (*AV1) isCodec() {
}
