package codecs

// H264 is a H264 codec.
type H264 struct {
	SPS []byte
	PPS []byte
}

// IsVideo returns whether the codec is a video one.
func (*H264) IsVideo() bool {
	return true
}

func (*H264) isCodec() {
}
