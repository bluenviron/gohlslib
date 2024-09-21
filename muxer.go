package gohlslib

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"

	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/gohlslib/v2/pkg/storage"
)

const (
	fmp4StartDTS            = 10 * time.Second
	mpegtsSegmentMinAUCount = 100
)

type switchableWriter struct {
	w io.Writer
}

func (w *switchableWriter) Write(p []byte) (int, error) {
	return w.w.Write(p)
}

// a prefix is needed to prevent usage of cached segments
// from previous muxing sessions.
func generatePrefix() (string, error) {
	var buf [6]byte
	_, err := rand.Read(buf[:])
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(buf[:]), nil
}

func mediaPlaylistPath(streamID string) string {
	return streamID + "_stream.m3u8"
}

func initFilePath(prefix string, streamID string) string {
	return prefix + "_" + streamID + "_init.mp4"
}

func segmentPath(prefix string, streamID string, segmentID uint64, mp4 bool) string {
	if mp4 {
		return prefix + "_" + streamID + "_seg" + strconv.FormatUint(segmentID, 10) + ".mp4"
	}
	return prefix + "_" + streamID + "_seg" + strconv.FormatUint(segmentID, 10) + ".ts"
}

func partPath(prefix string, streamID string, partID uint64) string {
	return prefix + "_" + streamID + "_part" + strconv.FormatUint(partID, 10) + ".mp4"
}

func fmp4TimeScale(c codecs.Codec) uint32 {
	switch codec := c.(type) {
	case *codecs.MPEG4Audio:
		return uint32(codec.SampleRate)

	case *codecs.Opus:
		return 48000
	}

	return 90000
}

type fmp4AugmentedSample struct {
	fmp4.PartSample
	dts time.Duration
	ntp time.Time
}

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
	// This is adjusted in order to include at least one IDR frame in each segment.
	// A player usually puts 3 segments in a buffer before reproducing the stream.
	// It defaults to 1sec.
	SegmentMinDuration time.Duration
	// Minimum duration of each part.
	// Parts are used in Low-Latency HLS in place of segments.
	// This is adjusted in order to produce segments with a similar duration.
	// A player usually puts 3 parts in a buffer before reproducing the stream.
	// It defaults to 200ms.
	PartMinDuration time.Duration
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

	mutex              sync.Mutex
	cond               *sync.Cond
	mtracks            []*muxerTrack
	mtracksByTrack     map[*Track]*muxerTrack
	streams            []*muxerStream
	prefix             string
	storageFactory     storage.Factory
	segmenter          *muxerSegmenter
	server             *muxerServer
	closed             bool
	nextSegmentID      uint64
	nextPartID         uint64 // low-latency only
	segmentDeleteCount int
}

// Start initializes the muxer.
func (m *Muxer) Start() error {
	if m.Variant == 0 {
		m.Variant = MuxerVariantLowLatency
	}
	if m.SegmentCount == 0 {
		m.SegmentCount = 7
	}
	if m.SegmentMinDuration == 0 {
		m.SegmentMinDuration = 1 * time.Second
	}
	if m.PartMinDuration == 0 {
		m.PartMinDuration = 200 * time.Millisecond
	}
	if m.SegmentMaxSize == 0 {
		m.SegmentMaxSize = 50 * 1024 * 1024
	}

	if m.VideoTrack == nil && m.AudioTrack == nil {
		return fmt.Errorf("one between VideoTrack and AudioTrack is required")
	}

	if m.Variant == MuxerVariantMPEGTS {
		if m.VideoTrack != nil {
			if _, ok := m.VideoTrack.Codec.(*codecs.H264); !ok {
				return fmt.Errorf(
					"the MPEG-TS variant of HLS supports H264 video only")
			}
		}
		if m.AudioTrack != nil {
			if _, ok := m.AudioTrack.Codec.(*codecs.MPEG4Audio); !ok {
				return fmt.Errorf(
					"the MPEG-TS variant of HLS supports MPEG-4 Audio only")
			}
		}
	}

	switch m.Variant {
	case MuxerVariantLowLatency:
		if m.SegmentCount < 7 {
			return fmt.Errorf("Low-Latency HLS requires at least 7 segments")
		}

	default:
		if m.SegmentCount < 3 {
			return fmt.Errorf("the minimum number of HLS segments is 3")
		}
	}

	m.cond = sync.NewCond(&m.mutex)
	m.mtracksByTrack = make(map[*Track]*muxerTrack)

	m.segmenter = &muxerSegmenter{
		muxer: m,
	}
	m.segmenter.initialize()

	m.server = &muxerServer{
		muxer: m,
	}
	m.server.initialize()

	if m.VideoTrack != nil {
		track := &muxerTrack{
			Track:     m.VideoTrack,
			variant:   m.Variant,
			isLeading: true,
		}
		track.initialize()
		m.mtracks = append(m.mtracks, track)
		m.mtracksByTrack[m.VideoTrack] = track
	}

	if m.AudioTrack != nil {
		track := &muxerTrack{
			Track:     m.AudioTrack,
			variant:   m.Variant,
			isLeading: m.VideoTrack == nil,
		}
		track.initialize()
		m.mtracks = append(m.mtracks, track)
		m.mtracksByTrack[m.AudioTrack] = track
	}

	if m.Variant == MuxerVariantMPEGTS {
		// nothing
	} else {
		// add initial gaps, required by iOS LL-HLS
		if m.Variant == MuxerVariantLowLatency {
			m.nextSegmentID = 7
		}
	}

	switch {
	case m.Variant == MuxerVariantMPEGTS:
		stream := &muxerStream{
			muxer:  m,
			tracks: m.mtracks,
			id:     "main",
		}
		stream.initialize()
		m.streams = append(m.streams, stream)

	default:
		if m.VideoTrack != nil {
			videoStream := &muxerStream{
				muxer:  m,
				tracks: []*muxerTrack{m.mtracksByTrack[m.VideoTrack]},
				id:     "video",
			}
			videoStream.initialize()
			m.streams = append(m.streams, videoStream)
		}

		if m.AudioTrack != nil {
			audioStream := &muxerStream{
				muxer:       m,
				tracks:      []*muxerTrack{m.mtracksByTrack[m.AudioTrack]},
				id:          "audio",
				isRendition: m.VideoTrack != nil,
			}
			audioStream.initialize()
			m.streams = append(m.streams, audioStream)
		}
	}

	var err error
	m.prefix, err = generatePrefix()
	if err != nil {
		return err
	}

	if m.Directory != "" {
		m.storageFactory = storage.NewFactoryDisk(m.Directory)
	} else {
		m.storageFactory = storage.NewFactoryRAM()
	}

	return nil
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.mutex.Lock()
	m.closed = true
	m.mutex.Unlock()

	m.cond.Broadcast()

	for _, stream := range m.streams {
		stream.close()
	}
}

// WriteAV1 writes an AV1 temporal unit.
func (m *Muxer) WriteAV1(
	ntp time.Time,
	pts time.Duration,
	tu [][]byte,
) error {
	return m.segmenter.writeAV1(ntp, pts, tu)
}

// WriteVP9 writes a VP9 frame.
func (m *Muxer) WriteVP9(
	ntp time.Time,
	pts time.Duration,
	frame []byte,
) error {
	return m.segmenter.writeVP9(ntp, pts, frame)
}

// WriteH265 writes an H265 access unit.
func (m *Muxer) WriteH265(
	ntp time.Time,
	pts time.Duration,
	au [][]byte,
) error {
	return m.segmenter.writeH265(ntp, pts, au)
}

// WriteH264 writes an H264 access unit.
func (m *Muxer) WriteH264(
	ntp time.Time,
	pts time.Duration,
	au [][]byte,
) error {
	return m.segmenter.writeH264(ntp, pts, au)
}

// WriteOpus writes Opus packets.
func (m *Muxer) WriteOpus(
	ntp time.Time,
	pts time.Duration,
	packets [][]byte,
) error {
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

func (m *Muxer) createFirstSegment(nextDTS time.Duration, nextNTP time.Time) error {
	for _, stream := range m.streams {
		err := stream.createFirstSegment(nextDTS, nextNTP)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Muxer) rotateParts(nextDTS time.Duration) error {
	m.mutex.Lock()
	err := m.rotatePartsInner(nextDTS, true)
	m.mutex.Unlock()

	if err != nil {
		return err
	}

	m.cond.Broadcast()

	return nil
}

func (m *Muxer) rotatePartsInner(nextDTS time.Duration, createNew bool) error {
	m.nextPartID++

	for _, stream := range m.streams {
		err := stream.rotateParts(nextDTS, createNew)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Muxer) rotateSegments(
	nextDTS time.Duration,
	nextNTP time.Time,
	force bool,
) error {
	m.mutex.Lock()
	err := m.rotateSegmentsInner(nextDTS, nextNTP, force)
	m.mutex.Unlock()

	if err != nil {
		return err
	}

	m.cond.Broadcast()

	return nil
}

func (m *Muxer) rotateSegmentsInner(
	nextDTS time.Duration,
	nextNTP time.Time,
	force bool,
) error {
	if m.Variant != MuxerVariantMPEGTS {
		err := m.rotatePartsInner(nextDTS, false)
		if err != nil {
			return err
		}
	}

	m.nextSegmentID++

	for _, stream := range m.streams {
		err := stream.rotateSegments(nextDTS, nextNTP, force)
		if err != nil {
			return err
		}
	}

	return nil
}
