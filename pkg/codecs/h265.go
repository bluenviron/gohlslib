package codecs

// H265 is a H265 codec.
type H265 struct {
	VPS []byte
	SPS []byte
	PPS []byte
}

// IsVideo returns whether the codec is a video one.
func (*H265) IsVideo() bool {
	return true
}

func (*H265) isCodec() {
}
