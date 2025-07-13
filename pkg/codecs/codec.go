// Package codecs contains codec definitions.
package codecs

// Codec is a codec.
type Codec interface {
	// IsVideo returns whether the codec is a video one.
	IsVideo() bool

	isCodec()
}
