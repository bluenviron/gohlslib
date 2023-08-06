package gohlslib

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/bluenviron/mediacommon/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"

	"github.com/bluenviron/gohlslib/pkg/codecs"
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
	server         *muxerServer
	segmenter      muxerSegmenter
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
					"the MPEG-TS variant of HLS only supports MPEG-4 Audio. Use the fMP4 or Low-Latency variants instead")
			}
		}
	}

	if m.Directory != "" {
		m.storageFactory = storage.NewFactoryDisk(m.Directory)
	} else {
		m.storageFactory = storage.NewFactoryRAM()
	}

	var err error
	m.server, err = newMuxerServer(
		m.Variant,
		m.SegmentCount,
		m.VideoTrack,
		m.AudioTrack,
		m.storageFactory,
	)
	if err != nil {
		return err
	}

	if m.Variant == MuxerVariantMPEGTS {
		m.segmenter = newMuxerSegmenterMPEGTS(
			m.SegmentDuration,
			m.SegmentMaxSize,
			m.VideoTrack,
			m.AudioTrack,
			m.storageFactory,
			m.server.onSegmentFinalized,
		)
	} else {
		m.segmenter = newMuxerSegmenterFMP4(
			m.Variant == MuxerVariantLowLatency,
			m.SegmentDuration,
			m.PartDuration,
			m.SegmentMaxSize,
			m.VideoTrack,
			m.AudioTrack,
			m.storageFactory,
			m.server.onSegmentFinalized,
			m.server.onPartFinalized,
		)
	}

	return nil
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.server.close()
	m.segmenter.close()
}

// WriteAV1 writes an AV1 OBU sequence.
func (m *Muxer) WriteAV1(ntp time.Time, pts time.Duration, obus [][]byte) error {
	codec := m.VideoTrack.Codec.(*codecs.AV1)
	update := false
	sequenceHeader := codec.SequenceHeader
	sequenceHeaderPresent := false

	for _, obu := range obus {
		var h av1.OBUHeader
		err := h.Unmarshal(obu)
		if err != nil {
			return err
		}

		if h.Type == av1.OBUTypeSequenceHeader {
			if !bytes.Equal(sequenceHeader, obu) {
				update = true
				sequenceHeader = obu
			}
			sequenceHeaderPresent = true
		}
	}

	if update {
		err := func() error {
			m.server.mutex.Lock()
			defer m.server.mutex.Unlock()
			codec.SequenceHeader = sequenceHeader
			return m.server.generateInitFile()
		}()
		if err != nil {
			return fmt.Errorf("unable to generate init.mp4: %v", err)
		}
		m.forceSwitch = true
	}

	forceSwitch := false
	if sequenceHeaderPresent && m.forceSwitch {
		m.forceSwitch = false
		forceSwitch = true
	}

	return m.segmenter.writeAV1(ntp, pts, obus, sequenceHeaderPresent, forceSwitch)
}

// WriteH26x writes an H264 or an H265 access unit.
func (m *Muxer) WriteH26x(ntp time.Time, pts time.Duration, au [][]byte) error {
	randomAccessPresent := false

	switch codec := m.VideoTrack.Codec.(type) {
	case *codecs.H265:
		update := false
		vps := codec.VPS
		sps := codec.SPS
		pps := codec.PPS

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
			err := func() error {
				m.server.mutex.Lock()
				defer m.server.mutex.Unlock()
				codec.VPS = vps
				codec.SPS = sps
				codec.PPS = pps
				return m.server.generateInitFile()
			}()
			if err != nil {
				return fmt.Errorf("unable to generate init.mp4: %v", err)
			}
			m.forceSwitch = true
		}

	case *codecs.H264:
		update := false
		nonIDRPresent := false
		sps := codec.SPS
		pps := codec.PPS

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
			err := func() error {
				m.server.mutex.Lock()
				defer m.server.mutex.Unlock()
				codec.SPS = sps
				codec.PPS = pps
				return m.server.generateInitFile()
			}()
			if err != nil {
				return fmt.Errorf("unable to generate init.mp4: %v", err)
			}
			m.forceSwitch = true
		}

		if !randomAccessPresent && !nonIDRPresent {
			return nil
		}
	}

	forceSwitch := false
	if randomAccessPresent && m.forceSwitch {
		m.forceSwitch = false
		forceSwitch = true
	}

	return m.segmenter.writeH26x(ntp, pts, au, randomAccessPresent, forceSwitch)
}

// WriteOpus writes Opus packets.
func (m *Muxer) WriteOpus(ntp time.Time, pts time.Duration, packets [][]byte) error {
	return m.segmenter.writeOpus(ntp, pts, packets)
}

// WriteMPEG4Audio writes MPEG-4 Audio access units.
func (m *Muxer) WriteMPEG4Audio(ntp time.Time, pts time.Duration, aus [][]byte) error {
	return m.segmenter.writeMPEG4Audio(ntp, pts, aus)
}

// Handle handles a HTTP request.
func (m *Muxer) Handle(w http.ResponseWriter, r *http.Request) {
	m.server.handle(w, r)
}
