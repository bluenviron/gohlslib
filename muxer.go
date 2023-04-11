package gohlslib

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"

	"github.com/bluenviron/gohlslib/pkg/codecparams"
	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/gohlslib/pkg/fmp4"
	"github.com/bluenviron/gohlslib/pkg/playlist"
	"github.com/bluenviron/gohlslib/pkg/storage"
)

// MuxerVariant is a muxer variant.
type MuxerVariant int

// supported variants.
const (
	MuxerVariantMPEGTS MuxerVariant = iota + 1
	MuxerVariantFMP4
	MuxerVariantLowLatency
)

// Muxer is a HLS muxer.
type Muxer struct {
	//
	// parameters (all optional except VideoTrack or AudioTrack).
	//
	// video track.
	VideoTrack *Track
	// audio track.
	AudioTrack *Track
	// Variant to use.
	// It defaults to MuxerVariantLowLatency
	Variant MuxerVariant
	// Number of HLS segments to keep on the server.
	// Segments allow to seek through the stream.
	// Their number doesn't influence latency.
	// It defaults to 7.
	SegmentCount int
	// Minimum duration of each segment.
	// A player usually puts 3 segments in a buffer before reproducing the stream.
	// The final segment duration is also influenced by the interval between IDR frames,
	// since the server changes the duration in order to include at least one IDR frame
	// in each segment.
	// It defaults to 1sec.
	SegmentDuration time.Duration
	// Minimum duration of each part.
	// Parts are used in Low-Latency HLS in place of segments.
	// A player usually puts 3 parts in a buffer before reproducing the stream.
	// Part duration is influenced by the distance between video/audio samples
	// and is adjusted in order to produce segments with a similar duration.
	// It defaults to 200ms.
	PartDuration time.Duration
	// Maximum size of each segment.
	// This prevents RAM exhaustion.
	// It defaults to 50MB.
	SegmentMaxSize uint64
	// Directory in which to save segments.
	// This decreases performance, since saving segments on disk is less performant
	// than saving them on RAM, but allows to preserve RAM.
	Directory string

	//
	// private
	//

	storageFactory storage.Factory
	mediaPlaylist  *muxerMediaPlaylist
	segmenter      muxerSegmenter
	mutex          sync.RWMutex
	closed         bool
	initStorage    storage.File
	forceSwitch    bool
}

// Start initializes the muxer.
func (m *Muxer) Start() error {
	if m.Variant == 0 {
		m.Variant = MuxerVariantLowLatency
	}
	if m.SegmentCount == 0 {
		m.SegmentCount = 7
	}
	if m.SegmentDuration == 0 {
		m.SegmentDuration = 1 * time.Second
	}
	if m.PartDuration == 0 {
		m.PartDuration = 200 * time.Millisecond
	}
	if m.SegmentMaxSize == 0 {
		m.SegmentMaxSize = 50 * 1024 * 1024
	}

	switch m.Variant {
	case MuxerVariantLowLatency:
		if m.SegmentCount < 7 {
			return fmt.Errorf("Low-Latency HLS requires at least 7 segments")
		}

	default:
		if m.SegmentCount < 3 {
			return fmt.Errorf("The minimum number of HLS segments is 3")
		}
	}

	if m.Variant == MuxerVariantMPEGTS {
		if m.VideoTrack != nil {
			if _, ok := m.VideoTrack.Codec.(*codecs.H264); !ok {
				return fmt.Errorf(
					"the MPEG-TS variant of HLS only supports H264 video. Use the fMP4 or Low-Latency variants instead")
			}
		}

		if m.AudioTrack != nil {
			if _, ok := m.AudioTrack.Codec.(*codecs.MPEG4Audio); !ok {
				return fmt.Errorf(
					"the MPEG-TS variant of HLS only supports MPEG4-audio. Use the fMP4 or Low-Latency variants instead")
			}
		}
	}

	if m.Directory != "" {
		m.storageFactory = storage.NewFactoryDisk(m.Directory)
	} else {
		m.storageFactory = storage.NewFactoryRAM()
	}

	m.mediaPlaylist = newMuxerMediaPlaylist(
		m.Variant,
		m.SegmentCount)

	if m.Variant == MuxerVariantMPEGTS {
		m.segmenter = newMuxerSegmenterMPEGTS(
			m.SegmentDuration,
			m.SegmentMaxSize,
			m.VideoTrack,
			m.AudioTrack,
			m.storageFactory,
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
			m.storageFactory,
			m.mediaPlaylist.onSegmentFinalized,
			m.mediaPlaylist.onPartFinalized,
		)
	}

	m.generateInitFile()

	return nil
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.mediaPlaylist.close()
	m.segmenter.close()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.closed = true

	if m.initStorage != nil {
		m.initStorage.Remove()
		m.initStorage = nil
	}
}

// WriteH26x writes an H264 or an H265 access unit.
func (m *Muxer) WriteH26x(ntp time.Time, pts time.Duration, au [][]byte) error {
	randomAccessPresent := false
	forceSwitch := false

	switch tcodec := m.VideoTrack.Codec.(type) {
	case *codecs.H264:
		nonIDRPresent := false
		sps := tcodec.SPS
		pps := tcodec.PPS
		update := false

		for _, nalu := range au {
			typ := h264.NALUType(nalu[0] & 0x1F)

			switch typ {
			case h264.NALUTypeIDR:
				randomAccessPresent = true

			case h264.NALUTypeNonIDR:
				nonIDRPresent = true

			case h264.NALUTypeSPS:
				if !bytes.Equal(sps, nalu) {
					update = true
					sps = nalu
				}

			case h264.NALUTypePPS:
				if !bytes.Equal(pps, nalu) {
					update = true
					pps = nalu
				}
			}
		}

		if update {
			m.mutex.Lock()
			tcodec.SPS = sps
			tcodec.PPS = pps
			m.generateInitFile()
			m.mutex.Unlock()
			m.forceSwitch = true
		}

		if !randomAccessPresent && !nonIDRPresent {
			return nil
		}

	case *codecs.H265:
		vps := tcodec.VPS
		sps := tcodec.SPS
		pps := tcodec.PPS
		update := false

		for _, nalu := range au {
			typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

			switch typ {
			case h265.NALUType_IDR_W_RADL, h265.NALUType_IDR_N_LP, h265.NALUType_CRA_NUT:
				randomAccessPresent = true

			case h265.NALUType_VPS_NUT:
				if !bytes.Equal(vps, nalu) {
					update = true
					vps = nalu
				}

			case h265.NALUType_SPS_NUT:
				if !bytes.Equal(sps, nalu) {
					update = true
					sps = nalu
				}

			case h265.NALUType_PPS_NUT:
				if !bytes.Equal(pps, nalu) {
					update = true
					pps = nalu
				}
			}
		}

		if update {
			m.mutex.Lock()
			tcodec.VPS = vps
			tcodec.SPS = sps
			tcodec.PPS = pps
			m.generateInitFile()
			m.mutex.Unlock()
			m.forceSwitch = true
		}
	}

	if randomAccessPresent && m.forceSwitch {
		m.forceSwitch = false
		forceSwitch = true
	}

	return m.segmenter.writeH26x(ntp, pts, au, randomAccessPresent, forceSwitch)
}

// WriteAudio writes an audio access unit.
func (m *Muxer) WriteAudio(ntp time.Time, pts time.Duration, au []byte) error {
	return m.segmenter.writeAudio(ntp, pts, au)
}

func queryVal(q url.Values, key string) string {
	vals, ok := q[key]
	if ok && len(vals) >= 1 {
		return vals[0]
	}
	return ""
}

// Handle handles a HTTP request.
func (m *Muxer) Handle(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path)
	q := r.URL.Query()
	msn := queryVal(q, "_HLS_msn")
	part := queryVal(q, "_HLS_part")
	skip := queryVal(q, "_HLS_skip")

	switch {
	case name == "index.m3u8":
		m.handleMultistreamPlaylist(w)

	case m.Variant != MuxerVariantMPEGTS && name == "init.mp4":
		m.handleInitFile(w)

	default:
		m.mediaPlaylist.handleFile(name, msn, part, skip, w)
	}
}

func (m *Muxer) handleMultistreamPlaylist(w http.ResponseWriter) {
	byts := func() []byte {
		m.mutex.RLock()
		defer m.mutex.RUnlock()

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
			switch tcodec := m.VideoTrack.Codec.(type) {
			case *codecs.H264:
				var sps h264.SPS
				err := sps.Unmarshal(tcodec.SPS)
				if err == nil {
					resolution = strconv.FormatInt(int64(sps.Width()), 10) + "x" + strconv.FormatInt(int64(sps.Height()), 10)

					f := sps.FPS()
					if f != 0 {
						frameRate = &f
					}
				}

			case *codecs.H265:
				var sps h265.SPS
				err := sps.Unmarshal(tcodec.SPS)
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
						codecs = append(codecs, codecparams.Marshal(m.VideoTrack.Codec))
					}
					if m.AudioTrack != nil {
						codecs = append(codecs, codecparams.Marshal(m.AudioTrack.Codec))
					}
					return codecs
				}(),
				Resolution: resolution,
				FrameRate:  frameRate,
				URI:        "stream.m3u8",
			}},
		}

		byts, err := p.Marshal()
		if err != nil {
			return nil
		}

		return byts
	}()

	if byts == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", `application/x-mpegURL`)
	w.WriteHeader(http.StatusOK)
	w.Write(byts)
}

func (m *Muxer) handleInitFile(w http.ResponseWriter) {
	r := func() io.ReadCloser {
		m.mutex.RLock()
		defer m.mutex.RUnlock()

		if m.initStorage == nil {
			return nil
		}

		r, err := m.initStorage.Reader()
		if err != nil {
			return nil
		}

		return r
	}()
	if r == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer r.Close()

	w.Header().Set("Content-Type", "video/mp4")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, r)
}

func (m *Muxer) generateInitFile() error {
	if m.Variant == MuxerVariantMPEGTS || m.closed {
		return nil
	}

	if m.initStorage != nil {
		m.initStorage.Remove()
		m.initStorage = nil
	}

	init := fmp4.Init{}
	trackID := 1

	if m.VideoTrack != nil {
		init.Tracks = append(init.Tracks, &fmp4.InitTrack{
			ID:        trackID,
			TimeScale: 90000,
			Codec:     m.VideoTrack.Codec,
		})
		trackID++
	}

	if m.AudioTrack != nil {
		init.Tracks = append(init.Tracks, &fmp4.InitTrack{
			ID:        trackID,
			TimeScale: m.segmenter.(*muxerSegmenterFMP4).audioTrackTimeScale,
			Codec:     m.AudioTrack.Codec,
		})
	}

	s, err := m.storageFactory.NewFile("init.mp4")
	if err != nil {
		return err
	}
	defer s.Finalize()

	part := s.NewPart()
	w := part.Writer()

	err = init.Marshal(w)
	if err != nil {
		return err
	}

	m.initStorage = s
	return nil
}
