// Package hls contains a HLS muxer and client.
package hls

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"

	"github.com/bluenviron/gohlslib/pkg/codecparams"
	"github.com/bluenviron/gohlslib/pkg/playlist"
	"github.com/bluenviron/gohlslib/pkg/storage"
)

// MuxerFileResponse is a response of the Muxer's File() func.
// Body must always be closed.
type MuxerFileResponse struct {
	Status int
	Header map[string]string
	Body   io.ReadCloser
}

// Muxer is a HLS muxer.
type Muxer struct {
	variant    muxerVariant
	fmp4       bool
	videoTrack format.Format
	audioTrack format.Format
}

// NewMuxer allocates a Muxer.
func NewMuxer(
	variant MuxerVariant,
	segmentCount int,
	segmentDuration time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack format.Format,
	audioTrack format.Format,
	dirPath string,
) (*Muxer, error) {
	var factory storage.Factory
	if dirPath != "" {
		factory = storage.NewFactoryDisk(dirPath)
	} else {
		factory = storage.NewFactoryRAM()
	}

	var v muxerVariant
	switch variant {
	case MuxerVariantMPEGTS:
		var err error
		v, err = newMuxerVariantMPEGTS(
			segmentCount,
			segmentDuration,
			segmentMaxSize,
			videoTrack,
			audioTrack,
			factory,
		)
		if err != nil {
			return nil, err
		}

	case MuxerVariantFMP4:
		v = newMuxerVariantFMP4(
			false,
			segmentCount,
			segmentDuration,
			partDuration,
			segmentMaxSize,
			videoTrack,
			audioTrack,
			factory,
		)

	default: // MuxerVariantLowLatency
		v = newMuxerVariantFMP4(
			true,
			segmentCount,
			segmentDuration,
			partDuration,
			segmentMaxSize,
			videoTrack,
			audioTrack,
			factory,
		)
	}

	return &Muxer{
		variant:    v,
		fmp4:       variant != MuxerVariantMPEGTS,
		videoTrack: videoTrack,
		audioTrack: audioTrack,
	}, nil
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.variant.close()
}

// WriteH26x writes an H264 or an H265 access unit.
func (m *Muxer) WriteH26x(ntp time.Time, pts time.Duration, au [][]byte) error {
	return m.variant.writeH26x(ntp, pts, au)
}

// WriteAudio writes an audio access unit.
func (m *Muxer) WriteAudio(ntp time.Time, pts time.Duration, au []byte) error {
	return m.variant.writeAudio(ntp, pts, au)
}

// File returns a file reader.
func (m *Muxer) File(name string, msn string, part string, skip string) *MuxerFileResponse {
	if name == "index.m3u8" {
		return m.multistreamPlaylist()
	}

	return m.variant.file(name, msn, part, skip)
}

func (m *Muxer) multistreamPlaylist() *MuxerFileResponse {
	return &MuxerFileResponse{
		Status: http.StatusOK,
		Header: map[string]string{
			"Content-Type": `application/x-mpegURL`,
		},
		Body: func() io.ReadCloser {
			p := &playlist.Multivariant{
				Version: func() int {
					if !m.fmp4 {
						return 3
					}
					return 9
				}(),
				IndependentSegments: true,
				Variants: []*playlist.MultivariantVariant{{
					Bandwidth: 200000,
					Codecs: func() []string {
						var codecs []string
						if m.videoTrack != nil {
							codecs = append(codecs, codecparams.Generate(m.videoTrack))
						}
						if m.audioTrack != nil {
							codecs = append(codecs, codecparams.Generate(m.audioTrack))
						}
						return codecs
					}(),
					URL: "stream.m3u8",
				}},
			}

			byts, _ := p.Marshal()

			return io.NopCloser(bytes.NewReader(byts))
		}(),
	}
}
