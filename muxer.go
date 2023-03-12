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
	// Variant to use.
	Variant MuxerVariant

	// Number of HLS segments to keep on the server.
	// Segments allow to seek through the stream.
	// Their number doesn't influence latency.
	SegmentCount int

	// Minimum duration of each segment.
	// A player usually puts 3 segments in a buffer before reproducing the stream.
	// The final segment duration is also influenced by the interval between IDR frames,
	// since the server changes the duration in order to include at least one IDR frame
	// in each segment.
	SegmentDuration time.Duration

	// Minimum duration of each part.
	// Parts are used in Low-Latency HLS in place of segments.
	// A player usually puts 3 parts in a buffer before reproducing the stream.
	// Part duration is influenced by the distance between video/audio samples
	// and is adjusted in order to produce segments with a similar duration.
	PartDuration time.Duration

	// Maximum size of each segment.
	// This prevents RAM exhaustion.
	SegmentMaxSize uint64

	// video track.
	VideoTrack format.Format

	// audio track.
	AudioTrack format.Format

	// (optional) directory in which to save segments.
	// This allows to save segments on disk instead on RAM.
	// This decreases performance, since disk is less performant than RAM,
	// but allows smaller RAM consumption.
	DirPath string

	variantImpl muxerVariantImpl
	fmp4        bool
}

// Start initializes the muxer.
func (m *Muxer) Start() error {
	var factory storage.Factory
	if m.DirPath != "" {
		factory = storage.NewFactoryDisk(m.DirPath)
	} else {
		factory = storage.NewFactoryRAM()
	}

	switch m.Variant {
	case MuxerVariantMPEGTS:
		var err error
		m.variantImpl, err = newMuxerVariantMPEGTS(
			m.SegmentCount,
			m.SegmentDuration,
			m.SegmentMaxSize,
			m.VideoTrack,
			m.AudioTrack,
			factory,
		)
		if err != nil {
			return err
		}

	case MuxerVariantFMP4:
		m.variantImpl = newMuxerVariantFMP4(
			false,
			m.SegmentCount,
			m.SegmentDuration,
			m.PartDuration,
			m.SegmentMaxSize,
			m.VideoTrack,
			m.AudioTrack,
			factory,
		)

	default: // MuxerVariantLowLatency
		m.variantImpl = newMuxerVariantFMP4(
			true,
			m.SegmentCount,
			m.SegmentDuration,
			m.PartDuration,
			m.SegmentMaxSize,
			m.VideoTrack,
			m.AudioTrack,
			factory,
		)
	}

	m.fmp4 = m.Variant != MuxerVariantMPEGTS

	return nil
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.variantImpl.close()
}

// WriteH26x writes an H264 or an H265 access unit.
func (m *Muxer) WriteH26x(ntp time.Time, pts time.Duration, au [][]byte) error {
	return m.variantImpl.writeH26x(ntp, pts, au)
}

// WriteAudio writes an audio access unit.
func (m *Muxer) WriteAudio(ntp time.Time, pts time.Duration, au []byte) error {
	return m.variantImpl.writeAudio(ntp, pts, au)
}

// File returns a file reader.
func (m *Muxer) File(name string, msn string, part string, skip string) *MuxerFileResponse {
	if name == "index.m3u8" {
		return m.multistreamPlaylist()
	}

	return m.variantImpl.file(name, msn, part, skip)
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
						if m.VideoTrack != nil {
							codecs = append(codecs, codecparams.Generate(m.VideoTrack))
						}
						if m.AudioTrack != nil {
							codecs = append(codecs, codecparams.Generate(m.AudioTrack))
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
