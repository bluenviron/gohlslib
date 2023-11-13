package gohlslib

import (
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"

	"github.com/vicon-security/gohlslib/pkg/codecparams"
	"github.com/vicon-security/gohlslib/pkg/codecs"
	"github.com/vicon-security/gohlslib/pkg/playlist"
	"github.com/vicon-security/gohlslib/pkg/storage"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
)

func boolPtr(v bool) *bool {
	return &v
}

func targetDuration(segments []muxerSegment) int {
	ret := int(0)

	// EXTINF, when rounded to the nearest integer, must be <= EXT-X-TARGETDURATION
	for _, sog := range segments {
		v := int(math.Round(sog.getDuration().Seconds()))
		if v > ret {
			ret = v
		}
	}

	return ret
}

func partTargetDuration(
	segments []muxerSegment,
	nextSegmentParts []*muxerPart,
) time.Duration {
	var ret time.Duration

	for _, sog := range segments {
		seg, ok := sog.(*muxerSegmentFMP4)
		if !ok {
			continue
		}

		for _, part := range seg.parts {
			if part.finalDuration > ret {
				ret = part.finalDuration
			}
		}
	}

	for _, part := range nextSegmentParts {
		if part.finalDuration > ret {
			ret = part.finalDuration
		}
	}

	// round to milliseconds to avoid changes, that are illegal on iOS
	return time.Millisecond * time.Duration(math.Ceil(float64(ret)/float64(time.Millisecond)))
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

func generateMultivariantPlaylist(
	variant MuxerVariant,
	videoTrack *Track,
	audioTrack *Track,
	segments []muxerSegment,
) ([]byte, error) {
	maxBandwidth, averageBandwidth := bandwidth(segments)
	var resolution string
	var frameRate *float64

	if videoTrack != nil {
		switch codec := videoTrack.Codec.(type) {
		case *codecs.AV1:
			var sh av1.SequenceHeader
			err := sh.Unmarshal(codec.SequenceHeader)
			if err != nil {
				return nil, err
			}

			resolution = strconv.FormatInt(int64(sh.Width()), 10) + "x" + strconv.FormatInt(int64(sh.Height()), 10)

			// TODO: FPS

		case *codecs.VP9:
			resolution = strconv.FormatInt(int64(codec.Width), 10) + "x" + strconv.FormatInt(int64(codec.Height), 10)

			// TODO: FPS

		case *codecs.H265:
			var sps h265.SPS
			err := sps.Unmarshal(codec.SPS)
			if err != nil {
				return nil, err
			}

			resolution = strconv.FormatInt(int64(sps.Width()), 10) + "x" + strconv.FormatInt(int64(sps.Height()), 10)

			f := sps.FPS()
			if f != 0 {
				frameRate = &f
			}

		case *codecs.H264:
			var sps h264.SPS
			err := sps.Unmarshal(codec.SPS)
			if err != nil {
				return nil, err
			}

			resolution = strconv.FormatInt(int64(sps.Width()), 10) + "x" + strconv.FormatInt(int64(sps.Height()), 10)

			f := sps.FPS()
			if f != 0 {
				frameRate = &f
			}
		}
	}

	pl := &playlist.Multivariant{
		Version: func() int {
			if variant == MuxerVariantMPEGTS {
				return 3
			}
			return 9
		}(),
		IndependentSegments: true,
		Variants: []*playlist.MultivariantVariant{{
			Bandwidth:        maxBandwidth,
			AverageBandwidth: &averageBandwidth,
			Codecs: func() []string {
				var codecs []string
				if videoTrack != nil {
					codecs = append(codecs, codecparams.Marshal(videoTrack.Codec))
				}
				if audioTrack != nil {
					codecs = append(codecs, codecparams.Marshal(audioTrack.Codec))
				}
				return codecs
			}(),
			Resolution: resolution,
			FrameRate:  frameRate,
			URI:        "stream.m3u8",
		}},
	}

	return pl.Marshal()
}

func generateInitFile(
	videoTrack *Track,
	audioTrack *Track,
	storageFactory storage.Factory,
	prefix string,
) (storage.File, error) {
	var init fmp4.Init
	trackID := 1

	if videoTrack != nil {
		init.Tracks = append(init.Tracks, &fmp4.InitTrack{
			ID:        trackID,
			TimeScale: 90000,
			Codec:     codecs.ToFMP4(videoTrack.Codec),
		})
		trackID++
	}

	if audioTrack != nil {
		init.Tracks = append(init.Tracks, &fmp4.InitTrack{
			ID:        trackID,
			TimeScale: fmp4TimeScale(audioTrack.Codec),
			Codec:     codecs.ToFMP4(audioTrack.Codec),
		})
	}

	f, err := storageFactory.NewFile(prefix + "_init.mp4")
	if err != nil {
		return nil, err
	}
	defer f.Finalize()

	part := f.NewPart()
	w := part.Writer()

	err = init.Marshal(w)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func generateMediaPlaylistMPEGTS(
	segments []muxerSegment,
	segmentDeleteCount int,
) ([]byte, error) {
	pl := &playlist.Media{
		Version:        3,
		AllowCache:     boolPtr(false),
		TargetDuration: targetDuration(segments),
		MediaSequence:  segmentDeleteCount,
	}

	for _, s := range segments {
		if seg, ok := s.(*muxerSegmentMPEGTS); ok {
			pl.Segments = append(pl.Segments, &playlist.MediaSegment{
				DateTime: &seg.startNTP,
				Duration: seg.getDuration(),
				URI:      seg.name,
			})
		}
	}

	return pl.Marshal()
}

func updateMediaPlaylistFMP4(
	segment muxerSegment,
	initFilename string,
	playlistFullPath string,
) {
	targetDuration := targetDuration([]muxerSegment{segment})
	fileExist := true

	data, err := os.ReadFile(playlistFullPath)

	if err != nil {
		fileExist = false
	}

	pl := &playlist.Media{
		Version:        9,
		TargetDuration: targetDuration,
		MediaSequence:  0,
		Endlist: true,
	}

	if fileExist {
		pl.Unmarshal(data)
	}

	pl.Map = &playlist.MediaMap{
		URI: initFilename + "_init.mp4",
	}

	switch seg := segment.(type) {
		case *muxerSegmentFMP4:
			plse := &playlist.MediaSegment{
				Duration: seg.getDuration(),
				URI:      seg.name,
			}

			pl.Segments = append(pl.Segments, plse)

		case *muxerGap:
			pl.Segments = append(pl.Segments, &playlist.MediaSegment{
				Gap:      true,
				Duration: seg.duration,
				URI:      "gap.mp4",
			})
	}

	updatedData, err := pl.Marshal()

	if err != nil {
		panic(err)
	}

	plFile, err := os.Create(playlistFullPath)

	if err != nil {
		panic(err)
	}

	if _, err = plFile.Write(updatedData); err != nil {
		panic(err)
	}

	err = plFile.Close()

	if err != nil {
		panic(err)
	}
}

func generateMediaPlaylistFMP4(
	isDeltaUpdate bool,
	variant MuxerVariant,
	segments []muxerSegment,
	nextSegmentParts []*muxerPart,
	nextPartID uint64,
	segmentDeleteCount int,
	prefix string,
) ([]byte, error) {
	targetDuration := targetDuration(segments)
	skipBoundary := time.Duration(targetDuration) * 6 * time.Second

	pl := &playlist.Media{
		Version:        9,
		TargetDuration: targetDuration,
		MediaSequence:  segmentDeleteCount,
		Endlist: true,
	}

	if variant == MuxerVariantLowLatency {
		partTarget := partTargetDuration(segments, nextSegmentParts)
		partHoldBack := (partTarget * 25) / 10

		pl.ServerControl = &playlist.MediaServerControl{
			CanBlockReload: true,
			PartHoldBack:   &partHoldBack,
			CanSkipUntil:   &skipBoundary,
		}

		pl.PartInf = &playlist.MediaPartInf{
			PartTarget: partTarget,
		}
	}

	skipped := 0

	if !isDeltaUpdate {
		pl.Map = &playlist.MediaMap{
			URI: prefix + "_init.mp4",
		}
	} else {
		var curDuration time.Duration
		shown := 0
		for _, segment := range segments {
			curDuration += segment.getDuration()
			if curDuration >= skipBoundary {
				break
			}
			shown++
		}
		skipped = len(segments) - shown

		pl.Skip = &playlist.MediaSkip{
			SkippedSegments: skipped,
		}
	}

	for i, sog := range segments {
		if i < skipped {
			continue
		}

		switch seg := sog.(type) {
		case *muxerSegmentFMP4:
			plse := &playlist.MediaSegment{
				Duration: seg.getDuration(),
				URI:      seg.name,
			}

			if (len(segments) - i) <= 2 {
				plse.DateTime = &seg.startNTP
			}

			if variant == MuxerVariantLowLatency && (len(segments)-i) <= 2 {
				for _, part := range seg.parts {
					plse.Parts = append(plse.Parts, &playlist.MediaPart{
						Duration:    part.finalDuration,
						URI:         part.getName(),
						Independent: part.isIndependent,
					})
				}
			}

			pl.Segments = append(pl.Segments, plse)

		case *muxerGap:
			pl.Segments = append(pl.Segments, &playlist.MediaSegment{
				Gap:      true,
				Duration: seg.duration,
				URI:      "gap.mp4",
			})
		}
	}

	if variant == MuxerVariantLowLatency {
		for _, part := range nextSegmentParts {
			pl.Parts = append(pl.Parts, &playlist.MediaPart{
				Duration:    part.finalDuration,
				URI:         part.getName(),
				Independent: part.isIndependent,
			})
		}

		// preload hint must always be present
		// otherwise hls.js goes into a loop
		pl.PreloadHint = &playlist.MediaPreloadHint{
			URI: partName(prefix, nextPartID),
		}
	}

	return pl.Marshal()
}

func generateMediaPlaylist(
	isDeltaUpdate bool,
	variant MuxerVariant,
	segments []muxerSegment,
	nextSegmentParts []*muxerPart,
	nextPartID uint64,
	segmentDeleteCount int,
	prefix string,
) ([]byte, error) {
	if variant == MuxerVariantMPEGTS {
		return generateMediaPlaylistMPEGTS(
			segments,
			segmentDeleteCount,
		)
	}

	return generateMediaPlaylistFMP4(
		isDeltaUpdate,
		variant,
		segments,
		nextSegmentParts,
		nextPartID,
		segmentDeleteCount,
		prefix,
	)
}

type muxerServer struct {
	variant        MuxerVariant
	segmentCount   int
	videoTrack     *Track
	audioTrack     *Track
	prefix         string
	storageFactory storage.Factory

	mutex                sync.Mutex
	cond                 *sync.Cond
	closed               bool
	segments             []muxerSegment
	segmentsByName       map[string]muxerSegment
	segmentDeleteCount   int
	partsByName          map[string]*muxerPart
	nextSegmentID        uint64
	nextSegmentParts     []*muxerPart
	nextPartID           uint64
	multivariantPlaylist []byte
	init                 storage.File

	playlistName				 		string
	removeStartupSegment		bool
	playlistMinutesInterval time.Duration
}

func newMuxerServer(
	variant MuxerVariant,
	segmentCount int,
	videoTrack *Track,
	audioTrack *Track,
	prefix string,
	storageFactory storage.Factory,
	playlistMinutesInterval time.Duration,
) *muxerServer {
	s := &muxerServer{
		variant:        variant,
		segmentCount:   segmentCount,
		videoTrack:     videoTrack,
		audioTrack:     audioTrack,
		prefix:         prefix,
		storageFactory: storageFactory,
		segmentsByName: make(map[string]muxerSegment),
		partsByName:    make(map[string]*muxerPart),
		playlistName:		time.Now().Format("2006-01-02T15:04:05.000Z"),
		removeStartupSegment: true,
		playlistMinutesInterval: playlistMinutesInterval,
	}

	s.cond = sync.NewCond(&s.mutex)

	return s
}

func (s *muxerServer) close() {
	func() {
		s.mutex.Lock()
		defer s.mutex.Unlock()
		s.closed = true
	}()

	s.cond.Broadcast()

	for _, segment := range s.segments {
		segment.close()
	}

	if s.init != nil {
		s.init.Remove()
	}
}

func (s *muxerServer) hasContent() bool {
	if s.variant == MuxerVariantFMP4 {
		return len(s.segments) >= 2
	}
	return len(s.segments) >= 1
}

func (s *muxerServer) hasPart(segmentID uint64, partID uint64) bool {
	if !s.hasContent() {
		return false
	}

	for _, sop := range s.segments {
		seg, ok := sop.(*muxerSegmentFMP4)
		if !ok {
			continue
		}

		if segmentID != seg.id {
			continue
		}

		// If the Client requests a Part Index greater than that of the final
		// Partial Segment of the Parent Segment, the Server MUST treat the
		// request as one for Part Index 0 of the following Parent Segment.
		if partID >= uint64(len(seg.parts)) {
			segmentID++
			partID = 0
			continue
		}

		return true
	}

	if segmentID != s.nextSegmentID {
		return false
	}

	if partID >= uint64(len(s.nextSegmentParts)) {
		return false
	}

	return true
}

func queryVal(q url.Values, key string) string {
	vals, ok := q[key]
	if ok && len(vals) >= 1 {
		return vals[0]
	}
	return ""
}

func (s *muxerServer) handle(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path)

	switch {
	case name == "index.m3u8":
		s.handleMultivariantPlaylist(w)

	case name == "stream.m3u8":
		q := r.URL.Query()
		msn := queryVal(q, "_HLS_msn")
		part := queryVal(q, "_HLS_part")
		skip := queryVal(q, "_HLS_skip")
		s.handleMediaPlaylist(msn, part, skip, w)

	case s.variant != MuxerVariantMPEGTS && name == s.prefix+"_init.mp4":
		s.handleInitFile(w)

	case (s.variant != MuxerVariantMPEGTS && strings.HasSuffix(name, ".mp4")) ||
		(s.variant == MuxerVariantMPEGTS && strings.HasSuffix(name, ".ts")):
		s.handleSegmentOrPart(name, w)
	}
}

func (s *muxerServer) handleMultivariantPlaylist(w http.ResponseWriter) {
	buf := func() []byte {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		for !s.closed && !s.hasContent() {
			s.cond.Wait()
		}

		return s.multivariantPlaylist
	}()

	if buf == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// allow caching but use a small period in order to
	// allow a stream to change tracks or bitrate
	w.Header().Set("Cache-Control", "max-age=30")
	w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
	w.WriteHeader(http.StatusOK)
	w.Write(buf)
}

func (s *muxerServer) handleMediaPlaylist(msn string, part string, skip string, w http.ResponseWriter) {
	isDeltaUpdate := false

	if s.variant == MuxerVariantLowLatency {
		isDeltaUpdate = skip == "YES" || skip == "v2"

		msnint, partint, err := parseMSNPart(msn, part)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if msn != "" {
			byts, err := func() ([]byte, error) {
				s.mutex.Lock()
				defer s.mutex.Unlock()

				// If the _HLS_msn is greater than the Media Sequence Number of the last
				// Media Segment in the current Playlist plus two, or if the _HLS_part
				// exceeds the last Partial Segment in the current Playlist by the
				// Advance Part Limit, then the server SHOULD immediately return Bad
				// Request, such as HTTP 400.
				if msnint > (s.nextSegmentID + 1) {
					w.WriteHeader(http.StatusBadRequest)
					return nil, nil
				}

				for !s.closed && !s.hasPart(msnint, partint) {
					s.cond.Wait()
				}

				return generateMediaPlaylist(
					isDeltaUpdate,
					s.variant,
					s.segments,
					s.nextSegmentParts,
					s.nextPartID,
					s.segmentDeleteCount,
					s.prefix,
				)
			}()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			if byts != nil {
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
				w.WriteHeader(http.StatusOK)
				w.Write(byts)
			}
			return
		}

		// part without msn is not supported.
		if part != "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	byts, err := func() ([]byte, error) {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		for !s.closed && !s.hasContent() {
			s.cond.Wait()
		}

		return generateMediaPlaylist(
			isDeltaUpdate,
			s.variant,
			s.segments,
			s.nextSegmentParts,
			s.nextPartID,
			s.segmentDeleteCount,
			s.prefix,
		)
	}()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if byts != nil {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
		w.WriteHeader(http.StatusOK)
		w.Write(byts)
	}
}

func (s *muxerServer) handleInitFile(w http.ResponseWriter) {
	init := func() storage.File {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		for !s.closed && !s.hasContent() {
			s.cond.Wait()
		}

		return s.init
	}()

	if init == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	r, err := init.Reader()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer r.Close()

	// allow caching but use a small period in order to
	// allow a stream to change track parameters
	w.Header().Set("Cache-Control", "max-age=30")
	w.Header().Set("Content-Type", "video/mp4")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, r)
}

func (s *muxerServer) handleSegmentOrPart(fname string, w http.ResponseWriter) {
	switch {
	case strings.HasPrefix(fname, s.prefix+"_"+"seg"):
		s.mutex.Lock()
		segment, ok := s.segmentsByName[fname]
		s.mutex.Unlock()

		if !ok {
			return
		}

		r, err := segment.reader()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer r.Close()

		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set(
			"Content-Type",
			func() string {
				if s.variant == MuxerVariantMPEGTS {
					return "video/MP2T"
				}
				return "video/mp4"
			}(),
		)
		w.WriteHeader(http.StatusOK)
		io.Copy(w, r)

	case s.variant == MuxerVariantLowLatency && strings.HasPrefix(fname, s.prefix+"_"+"part"):
		s.mutex.Lock()

		part := s.partsByName[fname]

		// support for EXT-X-PRELOAD-HINT
		if part == nil && fname == partName(s.prefix, s.nextPartID) {
			partID := s.nextPartID

			for !s.closed && s.nextPartID <= partID {
				s.cond.Wait()
			}

			part = s.partsByName[fname]
		}

		s.mutex.Unlock()

		if part == nil {
			return
		}

		r, err := part.reader()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer r.Close()

		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		io.Copy(w, r)
	}
}

func (s *muxerServer) publishPart(part *muxerPart) {
	func() {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		s.partsByName[part.getName()] = part
		s.nextSegmentParts = append(s.nextSegmentParts, part)
		s.nextPartID = part.id + 1
	}()

	s.cond.Broadcast()
}

func (s *muxerServer) publishSegment(segment muxerSegment) error {
	err := func() error {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		if s.removeStartupSegment {
			s.removeStartupSegment = false

			if toDeleteSeg, ok := segment.(*muxerSegmentFMP4); ok {
				for _, part := range toDeleteSeg.parts {
					delete(s.partsByName, part.getName())
				}
			}

			segment.close()

			return nil
		}

		// add initial gaps, required by iOS LL-HLS
		if s.variant == MuxerVariantLowLatency && len(s.segments) == 0 {
			for i := 0; i < 7; i++ {
				s.segments = append(s.segments, &muxerGap{
					duration: segment.getDuration(),
				})
			}
		}

		s.segmentsByName[segment.getName()] = segment
		s.segments = append(s.segments, segment)

		if seg, ok := segment.(*muxerSegmentFMP4); ok {
			s.nextSegmentID = seg.id + 1
		}

		s.nextSegmentParts = s.nextSegmentParts[:0]

		startSegName := s.segments[0].getName()
		startSegmentTime, _ := time.Parse("2006-01-02T15:04:05Z", startSegName[:len(startSegName)-4])
		startSegmentMinuteInterval := startSegmentTime.Truncate(s.playlistMinutesInterval)

		currentSegmentName := segment.getName()
		currentSegmentTime, _ := time.Parse("2006-01-02T15:04:05Z", currentSegmentName[:len(currentSegmentName)-4])
		currentSegmentMinuteInterval := currentSegmentTime.Truncate(s.playlistMinutesInterval)

		if currentSegmentMinuteInterval.Compare(startSegmentMinuteInterval) > 0 {
			segmentsToRemove := s.segments[:len(s.segments)-1]

			for _, seg := range segmentsToRemove {
				if toDeleteSeg, ok := seg.(*muxerSegmentFMP4); ok {
					for _, part := range toDeleteSeg.parts {
						delete(s.partsByName, part.getName())
					}
				}
				// seg.close() // This will actually delete the file, but we will have a separate job handling this
				delete(s.segmentsByName, seg.getName())
			}

			s.segments = s.segments[len(s.segments)-1:]
			// s.segmentDeleteCount++ // Removed as this only applies for live streams where we are dynamically adjusting the playlist
		}

		s.playlistName = currentSegmentMinuteInterval.Format("2006-01-02T15:04:05Z")
		playlistFullpath := s.storageFactory.GetPath() + "/" + s.playlistName + ".m3u8"

		if len(s.segments) == 1 {
			generateInitFile(s.videoTrack, s.audioTrack, s.storageFactory, s.playlistName)
		}

		updateMediaPlaylistFMP4(segment, s.playlistName, playlistFullpath)

		return nil
	}()
	if err != nil {
		return err
	}

	s.cond.Broadcast()
	return nil
}
