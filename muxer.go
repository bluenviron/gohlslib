package gohlslib

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h264"
	"github.com/aler9/gortsplib/v2/pkg/codecs/h265"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/orcaman/writerseeker"

	"github.com/bluenviron/gohlslib/pkg/codecparams"
	"github.com/bluenviron/gohlslib/pkg/fmp4"
	"github.com/bluenviron/gohlslib/pkg/playlist"
	"github.com/bluenviron/gohlslib/pkg/storage"
)

func extractVideoParams(track format.Format) [][]byte {
	switch ttrack := track.(type) {
	case *format.H264:
		params := make([][]byte, 2)
		params[0] = ttrack.SafeSPS()
		params[1] = ttrack.SafePPS()
		return params

	case *format.H265:
		params := make([][]byte, 3)
		params[0] = ttrack.SafeVPS()
		params[1] = ttrack.SafeSPS()
		params[2] = ttrack.SafePPS()
		return params

	default:
		return nil
	}
}

func videoParamsEqual(p1 [][]byte, p2 [][]byte) bool {
	if len(p1) != len(p2) {
		return true
	}

	for i, p := range p1 {
		if !bytes.Equal(p2[i], p) {
			return false
		}
	}
	return true
}

// MuxerVariant is a muxer variant.
type MuxerVariant int

// supported variants.
const (
	MuxerVariantMPEGTS MuxerVariant = iota + 1
	MuxerVariantFMP4
	MuxerVariantLowLatency
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
	// It defaults to MuxerVariantLowLatency
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
	// This decreases performance, since saving segments on disk is less performant
	// than saving them on RAM, but allows to preserve RAM.
	Directory string

	//
	// private
	//

	mediaPlaylist   *muxerMediaPlaylist
	segmenter       muxerSegmenter
	mutex           sync.Mutex
	lastVideoParams [][]byte
	initContent     []byte
}

// Start initializes the muxer.
func (m *Muxer) Start() error {
	if m.Variant == 0 {
		m.Variant = MuxerVariantLowLatency
	}

	var factory storage.Factory
	if m.Directory != "" {
		factory = storage.NewFactoryDisk(m.Directory)
	} else {
		factory = storage.NewFactoryRAM()
	}

	var videoTrackH264 *format.H264
	var audioTrackMPEG4Audio *format.MPEG4Audio

	if m.Variant == MuxerVariantMPEGTS {
		if m.VideoTrack != nil {
			var ok bool
			videoTrackH264, ok = m.VideoTrack.(*format.H264)
			if !ok {
				return fmt.Errorf(
					"the MPEG-TS variant of HLS only supports H264 video. Use the fMP4 or Low-Latency variants instead")
			}
		}

		if m.AudioTrack != nil {
			var ok bool
			audioTrackMPEG4Audio, ok = m.AudioTrack.(*format.MPEG4Audio)
			if !ok {
				return fmt.Errorf(
					"the MPEG-TS variant of HLS only supports MPEG4-audio. Use the fMP4 or Low-Latency variants instead")
			}
		}
	}

	m.mediaPlaylist = newMuxerMediaPlaylist(
		m.Variant,
		m.SegmentCount)

	if m.Variant == MuxerVariantMPEGTS {
		m.segmenter = newMuxerSegmenterMPEGTS(
			m.SegmentDuration,
			m.SegmentMaxSize,
			videoTrackH264,
			audioTrackMPEG4Audio,
			factory,
			m.mediaPlaylist.onSegmentFinalized,
		)
	} else {
		m.segmenter = newMuxerSegmenterFMP4(
			m.Variant == MuxerVariantLowLatency,
			m.SegmentCount,
			m.SegmentDuration,
			m.PartDuration,
			m.SegmentMaxSize,
			m.VideoTrack,
			m.AudioTrack,
			factory,
			m.mediaPlaylist.onSegmentFinalized,
			m.mediaPlaylist.onPartFinalized,
		)
	}

	return nil
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.mediaPlaylist.close()
	m.segmenter.close()
}

// WriteH26x writes an H264 or an H265 access unit.
func (m *Muxer) WriteH26x(ntp time.Time, pts time.Duration, au [][]byte) error {
	return m.segmenter.writeH26x(ntp, pts, au)
}

// WriteAudio writes an audio access unit.
func (m *Muxer) WriteAudio(ntp time.Time, pts time.Duration, au []byte) error {
	return m.segmenter.writeAudio(ntp, pts, au)
}

// File returns a file reader.
func (m *Muxer) File(name string, msn string, part string, skip string) *MuxerFileResponse {
	if name == "index.m3u8" {
		return m.multistreamPlaylist()
	}

	if m.Variant != MuxerVariantMPEGTS && name == "init.mp4" {
		return m.initFile()
	}

	return m.mediaPlaylist.file(name, msn, part, skip)
}

func (m *Muxer) multistreamPlaylist() *MuxerFileResponse {
	bandwidth, averageBandwidth := m.mediaPlaylist.bandwidth()

	if bandwidth == 0 {
		bandwidth = 200000
	}
	if averageBandwidth == 0 {
		averageBandwidth = 200000
	}

	var resolution string
	var frameRate *float64

	if m.VideoTrack != nil {
		switch ttrack := m.VideoTrack.(type) {
		case *format.H264:
			var sps h264.SPS
			err := sps.Unmarshal(ttrack.SafeSPS())
			if err == nil {
				resolution = strconv.FormatInt(int64(sps.Width()), 10) + "x" + strconv.FormatInt(int64(sps.Height()), 10)

				f := sps.FPS()
				if f != 0 {
					frameRate = &f
				}
			}

		case *format.H265:
			var sps h265.SPS
			err := sps.Unmarshal(ttrack.SafeSPS())
			if err == nil {
				resolution = strconv.FormatInt(int64(sps.Width()), 10) + "x" + strconv.FormatInt(int64(sps.Height()), 10)

				f := sps.FPS()
				if f != 0 {
					frameRate = &f
				}
			}
		}
	}

	p := &playlist.Multivariant{
		Version: func() int {
			if m.Variant == MuxerVariantMPEGTS {
				return 3
			}
			return 9
		}(),
		IndependentSegments: true,
		Variants: []*playlist.MultivariantVariant{{
			Bandwidth:        bandwidth,
			AverageBandwidth: &averageBandwidth,
			Codecs: func() []string {
				var codecs []string
				if m.VideoTrack != nil {
					codecs = append(codecs, codecparams.Marshal(m.VideoTrack))
				}
				if m.AudioTrack != nil {
					codecs = append(codecs, codecparams.Marshal(m.AudioTrack))
				}
				return codecs
			}(),
			Resolution: resolution,
			FrameRate:  frameRate,
			URI:        "stream.m3u8",
		}},
	}

	byts, _ := p.Marshal()

	return &MuxerFileResponse{
		Status: http.StatusOK,
		Header: map[string]string{
			"Content-Type": `application/x-mpegURL`,
		},
		Body: io.NopCloser(bytes.NewReader(byts)),
	}
}

func (m *Muxer) mustRegenerateInit() bool {
	if m.VideoTrack == nil {
		return false
	}

	videoParams := extractVideoParams(m.VideoTrack)
	if !videoParamsEqual(videoParams, m.lastVideoParams) {
		m.lastVideoParams = videoParams
		return true
	}

	return false
}

func (m *Muxer) initFile() *MuxerFileResponse {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.initContent == nil || m.mustRegenerateInit() {
		init := fmp4.Init{}
		trackID := 1

		if m.VideoTrack != nil {
			init.Tracks = append(init.Tracks, &fmp4.InitTrack{
				ID:        trackID,
				TimeScale: 90000,
				Format:    m.VideoTrack,
			})
			trackID++
		}

		if m.AudioTrack != nil {
			init.Tracks = append(init.Tracks, &fmp4.InitTrack{
				ID:        trackID,
				TimeScale: uint32(m.AudioTrack.ClockRate()),
				Format:    m.AudioTrack,
			})
		}

		buf := &writerseeker.WriterSeeker{}
		err := init.Marshal(buf)
		if err != nil {
			return &MuxerFileResponse{Status: http.StatusNotFound}
		}

		m.initContent = buf.Bytes()
	}

	return &MuxerFileResponse{
		Status: http.StatusOK,
		Header: map[string]string{
			"Content-Type": "video/mp4",
		},
		Body: io.NopCloser(bytes.NewReader(m.initContent)),
	}
}
