package codecs

// VP9 is a VP9 codec.
type VP9 struct {
	Width             int
	Height            int
	Profile           uint8
	BitDepth          uint8
	ChromaSubsampling uint8
	ColorRange        bool
}

// IsVideo returns whether the codec is a video one.
func (*VP9) IsVideo() bool {
	return true
}

func (*VP9) isCodec() {
}
