package gohlslib

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/gohlslib/v2/pkg/playlist"
	"github.com/bluenviron/gohlslib/v2/pkg/storage"
)

const (
	fmp4StartDTS               = 10 * time.Second
	mpegtsSegmentMinAUCount    = 100
	multivariantPlaylistMaxAge = "30"
	initMaxAge                 = "30"
	segmentMaxAge              = "3600"
)

func boolPtr(v bool) *bool {
	return &v
}

func parseMSNPart(msn string, part string) (uint64, uint64, error) {
	var msnint uint64
	if msn != "" {
		var err error
		msnint, err = strconv.ParseUint(msn, 10, 64)
		if err != nil {
			return 0, 0, err
		}
	}

	var partint uint64
	if part != "" {
		var err error
		partint, err = strconv.ParseUint(part, 10, 64)
		if err != nil {
			return 0, 0, err
		}
	}

	return msnint, partint, nil
}

func bandwidth(segments []muxerSegment) (int, int) {
	if len(segments) == 0 {
		return 0, 0
	}

	var maxBandwidth uint64
	var sizes uint64
	var durations time.Duration

	for _, seg := range segments {
		if _, ok := seg.(*muxerGap); !ok {
			bandwidth := 8 * seg.getSize() * uint64(time.Second) / uint64(seg.getDuration())
			if bandwidth > maxBandwidth {
				maxBandwidth = bandwidth
			}
			sizes += seg.getSize()
			durations += seg.getDuration()
		}
	}

	averageBandwidth := 8 * sizes * uint64(time.Second) / uint64(durations)

	return int(maxBandwidth), int(averageBandwidth)
}

func queryVal(q url.Values, key string) string {
	vals, ok := q[key]
	if ok && len(vals) >= 1 {
		return vals[0]
	}
	return ""
}

func areAllAudio(tracks []*muxerTrack) bool {
	for _, track := range tracks {
		if track.Codec.IsVideo() {
			return false
		}
	}
	return true
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
		return uint32(codec.Config.SampleRate)

	case *codecs.Opus:
		return 48000
	}

	return 90000
}

type switchableWriter struct {
	w io.Writer
}

func (w *switchableWriter) Write(p []byte) (int, error) {
	return w.w.Write(p)
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

	mutex          sync.Mutex
	cond           *sync.Cond
	mtracks        []*muxerTrack
	mtracksByTrack map[*Track]*muxerTrack
	streams        []*muxerStream
	leadingStream  *muxerStream
	prefix         string
	storageFactory storage.Factory
	segmenter      *muxerSegmenter
	server         *muxerServer
	closed         bool
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
			if track.Codec.IsVideo() {
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
			if track.Codec.IsVideo() {
				if hasVideo {
					return fmt.Errorf("only one video track is currently supported")
				}
				hasVideo = true
			} else {
				hasAudio = true //nolint:ineffassign,wastedassign
			}
		}
	}

	hasDefaultAudio := false

	for _, track := range m.Tracks {
		if !track.Codec.IsVideo() && track.IsDefault {
			if hasDefaultAudio {
				return fmt.Errorf("multiple default audio tracks are not supported")
			}
			hasDefaultAudio = true
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
		variant:            m.Variant,
		segmentMinDuration: m.SegmentMinDuration,
		partMinDuration:    m.PartMinDuration,
		parent:             m,
	}
	m.segmenter.initialize()

	m.server = &muxerServer{}
	m.server.initialize()

	m.server.registerPath("index.m3u8", m.handleMultivariantPlaylist)

	for i, track := range m.Tracks {
		mtrack := &muxerTrack{
			Track:     track,
			variant:   m.Variant,
			isLeading: track.Codec.IsVideo() || (!hasVideo && i == 0),
		}
		mtrack.initialize()
		m.mtracks = append(m.mtracks, mtrack)
		m.mtracksByTrack[track] = mtrack
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

	// add initial gaps, required by iOS LL-HLS
	nextSegmentID := uint64(0)
	if m.Variant == MuxerVariantLowLatency {
		nextSegmentID = 7
	}

	switch m.Variant {
	case MuxerVariantMPEGTS:
		stream := &muxerStream{
			isLeading:      true,
			variant:        m.Variant,
			segmentMaxSize: m.SegmentMaxSize,
			segmentCount:   m.SegmentCount,
			onEncodeError:  m.OnEncodeError,
			mutex:          &m.mutex,
			cond:           m.cond,
			prefix:         m.prefix,
			storageFactory: m.storageFactory,
			server:         m.server,
			tracks:         m.mtracks,
			id:             "main",
			nextSegmentID:  nextSegmentID,
		}
		err = stream.initialize()
		if err != nil {
			return err
		}
		m.streams = append(m.streams, stream)

	default:
		defaultAudioChosen := false

		for i, track := range m.mtracks {
			var id string
			if track.Codec.IsVideo() {
				id = "video" + strconv.FormatInt(int64(i+1), 10)
			} else {
				id = "audio" + strconv.FormatInt(int64(i+1), 10)
			}

			isRendition := !track.isLeading || (!track.Codec.IsVideo() && len(m.Tracks) > 1)
			isDefault := false
			name := ""

			if isRendition {
				if !hasDefaultAudio {
					if !defaultAudioChosen {
						defaultAudioChosen = true
						isDefault = true
					}
				} else {
					isDefault = track.IsDefault
				}

				if track.Name != "" {
					name = track.Name
				} else {
					name = id
				}
			}

			stream := &muxerStream{
				variant:        m.Variant,
				segmentMaxSize: m.SegmentMaxSize,
				segmentCount:   m.SegmentCount,
				onEncodeError:  m.OnEncodeError,
				mutex:          &m.mutex,
				cond:           m.cond,
				prefix:         m.prefix,
				storageFactory: m.storageFactory,
				server:         m.server,
				tracks:         []*muxerTrack{track},
				id:             id,
				isLeading:      track.isLeading,
				isRendition:    isRendition,
				name:           name,
				language:       track.Language,
				isDefault:      isDefault,
				nextSegmentID:  nextSegmentID,
			}
			err = stream.initialize()
			if err != nil {
				return err
			}
			m.streams = append(m.streams, stream)
		}
	}

	m.leadingStream = func() *muxerStream {
		for _, stream := range m.streams {
			if stream.isLeading {
				return stream
			}
		}
		return nil
	}()

	return nil
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.mutex.Lock()

	m.closed = true

	for _, stream := range m.streams {
		stream.close()
	}

	m.mutex.Unlock()

	m.cond.Broadcast()
}

// WriteAV1 writes an AV1 temporal unit.
func (m *Muxer) WriteAV1(
	track *Track,
	ntp time.Time,
	pts int64,
	tu [][]byte,
) error {
	return m.segmenter.writeAV1(m.mtracksByTrack[track], ntp, pts, tu)
}

// WriteVP9 writes a VP9 frame.
func (m *Muxer) WriteVP9(
	track *Track,
	ntp time.Time,
	pts int64,
	frame []byte,
) error {
	return m.segmenter.writeVP9(m.mtracksByTrack[track], ntp, pts, frame)
}

// WriteH265 writes an H265 access unit.
func (m *Muxer) WriteH265(
	track *Track,
	ntp time.Time,
	pts int64,
	au [][]byte,
) error {
	return m.segmenter.writeH265(m.mtracksByTrack[track], ntp, pts, au)
}

// WriteH264 writes an H264 access unit.
func (m *Muxer) WriteH264(
	track *Track,
	ntp time.Time,
	pts int64,
	au [][]byte,
) error {
	return m.segmenter.writeH264(m.mtracksByTrack[track], ntp, pts, au)
}

// WriteOpus writes Opus packets.
func (m *Muxer) WriteOpus(
	track *Track,
	ntp time.Time,
	pts int64,
	packets [][]byte,
) error {
	return m.segmenter.writeOpus(m.mtracksByTrack[track], ntp, pts, packets)
}

// WriteMPEG4Audio writes MPEG-4 Audio access units.
func (m *Muxer) WriteMPEG4Audio(
	track *Track,
	ntp time.Time,
	pts int64,
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
	err := m.rotatePartsInner(nextDTS)
	m.mutex.Unlock()

	if err != nil {
		return err
	}

	m.cond.Broadcast()

	return nil
}

func (m *Muxer) rotatePartsInner(nextDTS time.Duration) error {
	err := m.leadingStream.rotateParts(nextDTS, true)
	if err != nil {
		return err
	}

	for _, stream := range m.streams {
		if !stream.isLeading {
			err = stream.rotateParts(nextDTS, true)
			if err != nil {
				return err
			}
			stream.partTargetDuration = m.leadingStream.partTargetDuration
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
	err := m.leadingStream.rotateSegments(nextDTS, nextNTP, force)
	if err != nil {
		return err
	}

	for _, stream := range m.streams {
		if !stream.isLeading {
			err = stream.rotateSegments(nextDTS, nextNTP, force)
			if err != nil {
				return err
			}
			stream.targetDuration = m.leadingStream.targetDuration
			stream.partTargetDuration = m.leadingStream.partTargetDuration
		}
	}

	return nil
}

func (m *Muxer) handleMultivariantPlaylist(w http.ResponseWriter, r *http.Request) {
	buf := func() []byte {
		m.mutex.Lock()
		defer m.mutex.Unlock()

		for {
			if m.closed {
				return nil
			}

			if m.streams[0].hasContent() {
				break
			}

			m.cond.Wait()
		}

		buf, err := m.generateMultivariantPlaylist(r.URL.RawQuery)
		if err != nil {
			return nil
		}

		return buf
	}()

	if buf == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// allow caching but use a small period in order to
	// allow a stream to change tracks or bitrate
	w.Header().Set("Cache-Control", "max-age="+multivariantPlaylistMaxAge)
	w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
	w.WriteHeader(http.StatusOK)
	w.Write(buf)
}

func (m *Muxer) generateMultivariantPlaylist(rawQuery string) ([]byte, error) {
	// TODO: consider segments in all streams
	maxBandwidth, averageBandwidth := bandwidth(m.streams[0].segments)

	pl := &playlist.Multivariant{
		Version: func() int {
			if m.Variant == MuxerVariantMPEGTS {
				return 3
			}
			return 9
		}(),
		IndependentSegments: true,
		Variants: []*playlist.MultivariantVariant{{
			Bandwidth:        maxBandwidth,
			AverageBandwidth: &averageBandwidth,
		}},
	}

	for _, stream := range m.streams {
		err := stream.populateMultivariantPlaylist(pl, rawQuery)
		if err != nil {
			return nil, err
		}
	}

	return pl.Marshal()
}
