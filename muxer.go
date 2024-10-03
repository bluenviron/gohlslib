package gohlslib

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

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

func isVideo(codec codecs.Codec) bool {
	switch codec.(type) {
	case *codecs.AV1, *codecs.VP9, *codecs.H265, *codecs.H264:
		return true
	}
	return false
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

// MuxerOnEncodeErrorFunc is the prototype of Muxer.OnEncodeError.
type MuxerOnEncodeErrorFunc func(err error)

// Muxer is a HLS muxer.
type Muxer struct {
	//
	// parameters (all optional except Tracks).
	//
	// tracks.
	Tracks []*Track
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
	// callbacks (all optional)
	//
	// called when a non-fatal encode error occurs.
	OnEncodeError MuxerOnEncodeErrorFunc

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
	targetDuration     int
	partTargetDuration time.Duration
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
	if m.OnEncodeError == nil {
		m.OnEncodeError = func(e error) {
			log.Printf("%v", e)
		}
	}

	if len(m.Tracks) == 0 {
		return fmt.Errorf("at least one track must be provided")
	}

	hasVideo := false
	hasAudio := false

	if m.Variant == MuxerVariantMPEGTS {
		for _, track := range m.Tracks {
			if isVideo(track.Codec) {
				if hasVideo {
					return fmt.Errorf("the MPEG-TS variant of HLS supports a single video track only")
				}
				if _, ok := track.Codec.(*codecs.H264); !ok {
					return fmt.Errorf(
						"the MPEG-TS variant of HLS supports H264 video only")
				}
				hasVideo = true
			} else {
				if hasAudio {
					return fmt.Errorf("the MPEG-TS variant of HLS supports a single audio track only")
				}
				if _, ok := track.Codec.(*codecs.MPEG4Audio); !ok {
					return fmt.Errorf(
						"the MPEG-TS variant of HLS supports MPEG-4 Audio only")
				}
				hasAudio = true
			}
		}
	} else {
		for _, track := range m.Tracks {
			if isVideo(track.Codec) {
				if hasVideo {
					return fmt.Errorf("only one video track is currently supported")
				}
				hasVideo = true
			} else {
				hasAudio = true
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

	for i, track := range m.Tracks {
		mtrack := &muxerTrack{
			Track:     track,
			variant:   m.Variant,
			isLeading: isVideo(track.Codec) || (!hasVideo && i == 0),
		}
		mtrack.initialize()
		m.mtracks = append(m.mtracks, mtrack)
		m.mtracksByTrack[track] = mtrack
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
			muxer:     m,
			tracks:    m.mtracks,
			id:        "main",
			isLeading: true,
		}
		stream.initialize()
		m.streams = append(m.streams, stream)

	default:
		defaultRenditionChosen := false

		for i, track := range m.mtracks {
			var id string
			if isVideo(track.Codec) {
				id = "video" + strconv.FormatInt(int64(i+1), 10)
			} else {
				id = "audio" + strconv.FormatInt(int64(i+1), 10)
			}

			isRendition := !track.isLeading || (!isVideo(track.Codec) && len(m.Tracks) > 1)

			var isDefaultRendition bool
			if isRendition && !defaultRenditionChosen {
				isDefaultRendition = true
				defaultRenditionChosen = true
			} else {
				isDefaultRendition = false
			}

			stream := &muxerStream{
				muxer:              m,
				tracks:             []*muxerTrack{track},
				id:                 id,
				isLeading:          track.isLeading,
				isRendition:        isRendition,
				isDefaultRendition: isDefaultRendition,
			}
			stream.initialize()
			m.streams = append(m.streams, stream)
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
	track *Track,
	ntp time.Time,
	pts time.Duration,
	tu [][]byte,
) error {
	return m.segmenter.writeAV1(m.mtracksByTrack[track], ntp, pts, tu)
}

// WriteVP9 writes a VP9 frame.
func (m *Muxer) WriteVP9(
	track *Track,
	ntp time.Time,
	pts time.Duration,
	frame []byte,
) error {
	return m.segmenter.writeVP9(m.mtracksByTrack[track], ntp, pts, frame)
}

// WriteH265 writes an H265 access unit.
func (m *Muxer) WriteH265(
	track *Track,
	ntp time.Time,
	pts time.Duration,
	au [][]byte,
) error {
	return m.segmenter.writeH265(m.mtracksByTrack[track], ntp, pts, au)
}

// WriteH264 writes an H264 access unit.
func (m *Muxer) WriteH264(
	track *Track,
	ntp time.Time,
	pts time.Duration,
	au [][]byte,
) error {
	return m.segmenter.writeH264(m.mtracksByTrack[track], ntp, pts, au)
}

// WriteOpus writes Opus packets.
func (m *Muxer) WriteOpus(
	track *Track,
	ntp time.Time,
	pts time.Duration,
	packets [][]byte,
) error {
	return m.segmenter.writeOpus(m.mtracksByTrack[track], ntp, pts, packets)
}

// WriteMPEG4Audio writes MPEG-4 Audio access units.
func (m *Muxer) WriteMPEG4Audio(
	track *Track,
	ntp time.Time,
	pts time.Duration,
	aus [][]byte,
) error {
	return m.segmenter.writeMPEG4Audio(m.mtracksByTrack[track], ntp, pts, aus)
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
