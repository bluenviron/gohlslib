package codecs

// KLV is a KLV codec.
type KLV struct {
	// whether the KLV stream is synchronous (has its own PES timestamps)
	Synchronous bool
}

// IsVideo implements Codec.
func (*KLV) IsVideo() bool {
	return false
}

func (*KLV) isCodec() {}
