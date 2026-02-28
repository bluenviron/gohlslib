package gohlslib

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/codecparams"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/gohlslib/v2/pkg/playlist"
	"github.com/bluenviron/gohlslib/v2/pkg/storage"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
)

func filterOutHLSParams(rawQuery string) string {
	if rawQuery != "" {
		if q, err := url.ParseQuery(rawQuery); err == nil {
			for k := range q {
				if strings.HasPrefix(k, "_HLS_") {
					delete(q, k)
				}
			}
			rawQuery = q.Encode()
		}
	}
	return rawQuery
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

	for _, seg := range segments {
		seg, ok := seg.(*muxerSegmentFMP4)
		if !ok {
			continue
		}

		for _, part := range seg.parts {
			if part.getDuration() > ret {
				ret = part.getDuration()
			}
		}
	}

	for _, part := range nextSegmentParts {
		if part.getDuration() > ret {
			ret = part.getDuration()
		}
	}

	// round to milliseconds to minimize changes
	return time.Millisecond * time.Duration(math.Ceil(float64(ret)/float64(time.Millisecond)))
}

type generateMediaPlaylistFunc func(
	isDeltaUpdate bool,
	rawQuery string,
) ([]byte, error)

type muxerStream struct {
	variant        MuxerVariant
	segmentMaxSize uint64
	segmentCount   int
	onEncodeError  MuxerOnEncodeErrorFunc
	mutex          *sync.Mutex
	cond           *sync.Cond
	prefix         string
	storageFactory storage.Factory
	server         *muxerServer
	tracks         []*muxerTrack
	id             string
	isLeading      bool
	isRendition    bool
	name           string
	language       string
	isDefault      bool
	nextSegmentID  uint64
	nextPartID     uint64

	generateMediaPlaylist  generateMediaPlaylistFunc
	mpegtsSwitchableWriter *switchableWriter // mpegts only
	mpegtsWriter           *mpegts.Writer    // mpegts only
	segments               []muxerSegment
	nextSegment            muxerSegment
	nextPart               *muxerPart // low-latency only
	initFilePresent        bool       // fmp4 only
	segmentDeleteCount     int
	closed                 bool
	targetDuration         int
	partTargetDuration     time.Duration
}

func (s *muxerStream) initialize() error {
	for _, track := range s.tracks {
		track.stream = s
	}

	if s.variant == MuxerVariantMPEGTS {
		s.generateMediaPlaylist = s.generateMediaPlaylistMPEGTS

		tracks := make([]*mpegts.Track, len(s.tracks))
		for i, track := range s.tracks {
			tracks[i] = track.mpegtsTrack
		}
		s.mpegtsSwitchableWriter = &switchableWriter{}
		s.mpegtsWriter = &mpegts.Writer{W: s.mpegtsSwitchableWriter, Tracks: tracks}
		err := s.mpegtsWriter.Initialize()
		if err != nil {
			return err
		}
	} else {
		s.generateMediaPlaylist = s.generateMediaPlaylistFMP4
	}

	s.server.registerPath(mediaPlaylistPath(s.id), s.handleMediaPlaylist)
	return nil
}

func (s *muxerStream) close() {
	s.closed = true

	for _, segment := range s.segments {
		segment.close()
	}

	if s.nextPart != nil {
		s.nextPart.finalize(0) //nolint:errcheck
	}

	if s.nextSegment != nil {
		s.nextSegment.finalize(0) //nolint:errcheck
		s.nextSegment.close()
	}
}

func (s *muxerStream) populateMultivariantPlaylist(
	pl *playlist.Multivariant,
	rawQuery string,
) error {
	mv := pl.Variants[0]

	for _, track := range s.tracks {
		codec := codecparams.Marshal(track.Codec)
		if !slices.Contains(mv.Codecs, codec) {
			mv.Codecs = append(mv.Codecs, codec)
		}

		switch codec := track.Codec.(type) {
		case *codecs.AV1:
			var sh av1.SequenceHeader
			err := sh.Unmarshal(codec.SequenceHeader)
			if err != nil {
				return err
			}

			mv.Resolution = strconv.FormatInt(int64(sh.Width()), 10) + "x" + strconv.FormatInt(int64(sh.Height()), 10)

			// TODO: FPS

		case *codecs.VP9:
			mv.Resolution = strconv.FormatInt(int64(codec.Width), 10) + "x" + strconv.FormatInt(int64(codec.Height), 10)

			// TODO: FPS

		case *codecs.H265:
			var sps h265.SPS
			err := sps.Unmarshal(codec.SPS)
			if err != nil {
				return err
			}

			mv.Resolution = strconv.FormatInt(int64(sps.Width()), 10) + "x" + strconv.FormatInt(int64(sps.Height()), 10)

			f := sps.FPS()
			if f != 0 {
				mv.FrameRate = &f
			}

		case *codecs.H264:
			var sps h264.SPS
			err := sps.Unmarshal(codec.SPS)
			if err != nil {
				return err
			}

			mv.Resolution = strconv.FormatInt(int64(sps.Width()), 10) + "x" + strconv.FormatInt(int64(sps.Height()), 10)

			f := sps.FPS()
			if f != 0 {
				mv.FrameRate = &f
			}
		}
	}

	uri := mediaPlaylistPath(s.id)
	if rawQuery != "" {
		uri += "?" + rawQuery
	}

	if s.isLeading {
		mv.URI = uri
	}

	if s.isRendition {
		mv.Audio = "audio"

		r := &playlist.MultivariantRendition{
			Type:       playlist.MultivariantRenditionTypeAudio,
			GroupID:    "audio",
			Name:       s.name,
			Language:   s.language,
			Autoselect: true,
			Default:    s.isDefault,
		}

		// draft-pantos-hls-rfc8216bis:
		// If the media type is VIDEO or AUDIO, a missing URI attribute
		// indicates that the media data for this Rendition is included in the
		// Media Playlist of any EXT-X-STREAM-INF tag referencing this EXT-
		// X-MEDIA tag.
		if !s.isLeading {
			r.URI = &uri
		}

		pl.Renditions = append(pl.Renditions, r)
	}

	return nil
}

func (s *muxerStream) hasContent() bool {
	if s.variant == MuxerVariantFMP4 {
		return len(s.segments) >= 2
	}
	return len(s.segments) >= 1
}

func (s *muxerStream) hasPart(segmentID uint64, partID uint64) bool {
	if segmentID == s.nextSegmentID {
		if partID < uint64(len(s.nextSegment.(*muxerSegmentFMP4).parts)) {
			return true
		}
	} else {
		for _, sop := range s.segments {
			if seg, ok := sop.(*muxerSegmentFMP4); ok && segmentID == seg.id {
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
		}
	}

	return false
}

func (s *muxerStream) handleMediaPlaylist(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	msn := queryVal(q, "_HLS_msn")
	part := queryVal(q, "_HLS_part")
	skip := queryVal(q, "_HLS_skip")

	isDeltaUpdate := false

	if s.variant == MuxerVariantLowLatency {
		isDeltaUpdate = skip == "YES" || skip == "v2"

		msnint, partint, err := parseMSNPart(msn, part)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		switch {
		case msn != "":
			byts := func() []byte {
				s.mutex.Lock()
				defer s.mutex.Unlock()

				for {
					if s.closed {
						w.WriteHeader(http.StatusInternalServerError)
						return nil
					}

					// If the _HLS_msn is greater than the Media Sequence Number of the last
					// Media Segment in the current Playlist plus two, or if the _HLS_part
					// exceeds the last Partial Segment in the current Playlist by the
					// Advance Part Limit, then the server SHOULD immediately return Bad
					// Request, such as HTTP 400.
					if msnint > (s.nextSegmentID+1) || msnint < (s.nextSegmentID-uint64(len(s.segments)-1)) {
						w.WriteHeader(http.StatusBadRequest)
						return nil
					}

					if s.hasContent() && s.hasPart(msnint, partint) {
						break
					}

					s.cond.Wait()
				}

				var byts []byte
				byts, err = s.generateMediaPlaylist(
					isDeltaUpdate,
					r.URL.RawQuery,
				)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return nil
				}

				return byts
			}()

			if byts != nil {
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
				w.WriteHeader(http.StatusOK)
				w.Write(byts)
			}
			return

		case part != "": // part without msn is not supported.
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	byts := func() []byte {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		for {
			if s.closed {
				w.WriteHeader(http.StatusInternalServerError)
				return nil
			}

			if s.hasContent() {
				break
			}

			s.cond.Wait()
		}

		byts, err := s.generateMediaPlaylist(
			isDeltaUpdate,
			r.URL.RawQuery,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return nil
		}

		return byts
	}()

	if byts != nil {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
		w.WriteHeader(http.StatusOK)
		w.Write(byts)
	}
}

func (s *muxerStream) generateMediaPlaylistMPEGTS(
	_ bool,
	rawQuery string,
) ([]byte, error) {
	pl := &playlist.Media{
		Version:        3,
		AllowCache:     ptrOf(false),
		TargetDuration: s.targetDuration,
		MediaSequence:  s.segmentDeleteCount,
	}

	for _, s := range s.segments {
		if seg, ok := s.(*muxerSegmentMPEGTS); ok {
			uri := seg.path
			if rawQuery != "" {
				uri += "?" + rawQuery
			}

			pl.Segments = append(pl.Segments, &playlist.MediaSegment{
				DateTime: &seg.startNTP,
				Duration: seg.getDuration(),
				URI:      uri,
			})
		}
	}

	return pl.Marshal()
}

func (s *muxerStream) generateMediaPlaylistFMP4(
	isDeltaUpdate bool,
	rawQuery string,
) ([]byte, error) {
	skipBoundary := time.Duration(s.targetDuration) * 6 * time.Second
	rawQuery = filterOutHLSParams(rawQuery)

	pl := &playlist.Media{
		Version:        10,
		TargetDuration: s.targetDuration,
		MediaSequence:  s.segmentDeleteCount,
	}

	if s.variant == MuxerVariantLowLatency {
		partHoldBack := (s.partTargetDuration * 25) / 10

		pl.ServerControl = &playlist.MediaServerControl{
			CanBlockReload: true,
			PartHoldBack:   &partHoldBack,
			CanSkipUntil:   &skipBoundary,
		}

		pl.PartInf = &playlist.MediaPartInf{
			PartTarget: s.partTargetDuration,
		}
	}

	skipped := 0

	if !isDeltaUpdate {
		uri := initFilePath(s.prefix, s.id)
		if rawQuery != "" {
			uri += "?" + rawQuery
		}

		pl.Map = &playlist.MediaMap{
			URI: uri,
		}
	} else {
		var curDuration time.Duration
		shown := 0
		for _, segment := range s.segments {
			curDuration += segment.getDuration()
			if curDuration >= skipBoundary {
				break
			}
			shown++
		}
		skipped = len(s.segments) - shown

		pl.Skip = &playlist.MediaSkip{
			SkippedSegments: skipped,
		}
	}

	for i, sog := range s.segments {
		if i < skipped {
			continue
		}

		switch seg := sog.(type) {
		case *muxerSegmentFMP4:
			u := seg.path
			if rawQuery != "" {
				u += "?" + rawQuery
			}

			plse := &playlist.MediaSegment{
				Duration: seg.getDuration(),
				URI:      u,
			}

			if (len(s.segments) - i) <= 2 {
				plse.DateTime = &seg.startNTP
			}

			if s.variant == MuxerVariantLowLatency && (len(s.segments)-i) <= 2 {
				for _, part := range seg.parts {
					u = part.path
					if rawQuery != "" {
						u += "?" + rawQuery
					}

					plse.Parts = append(plse.Parts, &playlist.MediaPart{
						Duration:    part.getDuration(),
						URI:         u,
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

	if s.variant == MuxerVariantLowLatency {
		for _, part := range s.nextSegment.(*muxerSegmentFMP4).parts {
			u := part.path
			if rawQuery != "" {
				u += "?" + rawQuery
			}

			pl.Parts = append(pl.Parts, &playlist.MediaPart{
				Duration:    part.getDuration(),
				URI:         u,
				Independent: part.isIndependent,
			})
		}

		// preload hint must always be present
		// otherwise hls.js goes into a loop
		uri := partPath(s.prefix, s.id, s.nextPartID)
		if rawQuery != "" {
			uri += "?" + rawQuery
		}
		pl.PreloadHint = &playlist.MediaPreloadHint{
			URI: uri,
		}
	}

	return pl.Marshal()
}

func (s *muxerStream) generateAndCacheInitFile() error {
	var init fmp4.Init
	trackID := 1

	for _, track := range s.tracks {
		init.Tracks = append(init.Tracks, &fmp4.InitTrack{
			ID:        trackID,
			TimeScale: fmp4TimeScale(track.Codec),
			Codec:     toFMP4(track.Codec),
		})
		trackID++
	}

	var w seekablebuffer.Buffer
	err := init.Marshal(&w)
	if err != nil {
		return err
	}

	s.initFilePresent = true
	initFile := w.Bytes()

	var contentType string
	if areAllAudio(s.tracks) {
		contentType = "audio/mp4"
	} else {
		contentType = "video/mp4"
	}

	s.server.registerPath(
		initFilePath(s.prefix, s.id),
		func(w http.ResponseWriter, _ *http.Request) {
			// allow caching but use a small period in order to
			// allow a stream to change track parameters
			w.Header().Set("Cache-Control", "max-age="+initMaxAge)
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(http.StatusOK)
			w.Write(initFile)
		})

	return nil
}

func (s *muxerStream) createFirstSegment(
	nextDTS time.Duration,
	nextNTP time.Time,
) error {
	if s.variant == MuxerVariantMPEGTS { //nolint:dupl
		seg := &muxerSegmentMPEGTS{
			segmentMaxSize: s.segmentMaxSize,
			prefix:         s.prefix,
			storageFactory: s.storageFactory,
			streamID:       s.id,
			mpegtsWriter:   s.mpegtsWriter,
			id:             s.nextSegmentID,
			startNTP:       nextNTP,
			startDTS:       nextDTS,
		}
		err := seg.initialize()
		if err != nil {
			return err
		}
		s.nextSegment = seg

		s.mpegtsSwitchableWriter.w = seg.bw
	} else {
		seg := &muxerSegmentFMP4{
			prefix:             s.prefix,
			storageFactory:     s.storageFactory,
			streamID:           s.id,
			id:                 s.nextSegmentID,
			startNTP:           nextNTP,
			startDTS:           nextDTS,
			fromForcedRotation: false,
		}
		err := seg.initialize()
		if err != nil {
			return err
		}
		s.nextSegment = seg

		s.nextPart = &muxerPart{
			segmentMaxSize: s.segmentMaxSize,
			streamID:       s.id,
			streamTracks:   s.tracks,
			segment:        seg,
			startDTS:       seg.startDTS,
			prefix:         seg.prefix,
			id:             s.nextPartID,
			storage:        seg.storage.NewPart(),
		}
		s.nextPart.initialize()
	}

	return nil
}

func (s *muxerStream) rotateParts(
	nextDTS time.Duration,
	createNew bool,
) error {
	s.nextPartID++

	part := s.nextPart
	s.nextPart = nil

	err := part.finalize(nextDTS)
	if err != nil {
		return err
	}

	if s.variant == MuxerVariantLowLatency {
		part.segment.parts = append(part.segment.parts, part)

		s.server.registerPath(
			part.path,
			func(w http.ResponseWriter, _ *http.Request) {
				r, err2 := part.reader()
				if err2 != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				defer r.Close()

				var contentType string
				if areAllAudio(s.tracks) {
					contentType = "audio/mp4"
				} else {
					contentType = "video/mp4"
				}

				w.Header().Set("Cache-Control", "max-age="+segmentMaxAge)
				w.Header().Set("Content-Type", contentType)
				w.WriteHeader(http.StatusOK)
				io.Copy(w, r)
			})

		// EXT-X-PRELOAD-HINT
		capturePartID := s.nextPartID
		partPath := partPath(s.prefix, s.id, capturePartID)
		s.server.registerPath(
			partPath,
			func(w http.ResponseWriter, r *http.Request) {
				s.mutex.Lock()

				for {
					if s.closed {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}

					if s.nextPartID > capturePartID {
						break
					}

					s.cond.Wait()
				}

				h := s.server.getPathHandler(partPath)

				s.mutex.Unlock()

				if h != nil {
					h(w, r)
				}
			})
	}

	if createNew {
		nextPart := &muxerPart{
			segmentMaxSize: s.segmentMaxSize,
			streamID:       s.id,
			streamTracks:   s.tracks,
			segment:        part.segment,
			startDTS:       nextDTS,
			prefix:         s.prefix,
			id:             s.nextPartID,
			storage:        part.segment.storage.NewPart(),
		}
		nextPart.initialize()
		s.nextPart = nextPart
	}

	// while segment target duration can be increased indefinitely,
	// part target duration cannot, since
	// "The duration of a Partial Segment MUST be at least 85% of the Part Target Duration"
	// so it's better to reset it every time.
	if s.isLeading {
		partTargetDuration := partTargetDuration(s.segments, s.nextSegment.(*muxerSegmentFMP4).parts)
		if s.partTargetDuration == 0 {
			s.partTargetDuration = partTargetDuration
		} else if partTargetDuration != s.partTargetDuration {
			s.onEncodeError(fmt.Errorf("part duration changed from %v to %v - this will cause an error in iOS clients",
				s.partTargetDuration, partTargetDuration))
			s.partTargetDuration = partTargetDuration
		}
	}

	return nil
}

func (s *muxerStream) rotateSegments(
	nextDTS time.Duration,
	nextNTP time.Time,
	force bool,
) error {
	if s.variant != MuxerVariantMPEGTS {
		err := s.rotateParts(nextDTS, false)
		if err != nil {
			return err
		}
	}

	s.nextSegmentID++

	segment := s.nextSegment
	s.nextSegment = nil

	err := segment.finalize(nextDTS)
	if err != nil {
		segment.close()
		return err
	}

	// add initial gaps, required by iOS LL-HLS
	if s.variant == MuxerVariantLowLatency && len(s.segments) == 0 {
		for range 7 {
			s.segments = append(s.segments, &muxerGap{
				duration: segment.getDuration(),
			})
		}
	}

	s.segments = append(s.segments, segment)

	s.server.registerPath(
		segment.getPath(),
		func(w http.ResponseWriter, _ *http.Request) {
			r, err2 := segment.reader()
			if err2 != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			defer r.Close()

			var contentType string
			switch {
			case s.variant == MuxerVariantMPEGTS:
				contentType = "video/mp2t"

			case areAllAudio(s.tracks):
				contentType = "audio/mp4"

			default:
				contentType = "video/mp4"
			}

			w.Header().Set("Cache-Control", "max-age="+segmentMaxAge)
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(http.StatusOK)
			io.Copy(w, r)
		})

	// delete old segments and parts
	if len(s.segments) > s.segmentCount {
		toDelete := s.segments[0]

		if toDeleteSeg, ok := toDelete.(*muxerSegmentFMP4); ok {
			for _, part := range toDeleteSeg.parts {
				s.server.unregisterPath(part.path)
			}
		}

		toDelete.close()

		s.server.unregisterPath(toDelete.getPath())

		s.segments = s.segments[1:]

		s.segmentDeleteCount++
	}

	// regenerate init files only if missing or codec parameters have changed
	if s.variant != MuxerVariantMPEGTS && (!s.initFilePresent || segment.isFromForcedRotation()) {
		err = s.generateAndCacheInitFile()
		if err != nil {
			return err
		}
	}

	if s.variant == MuxerVariantMPEGTS { //nolint:dupl
		seg := &muxerSegmentMPEGTS{
			segmentMaxSize: s.segmentMaxSize,
			prefix:         s.prefix,
			storageFactory: s.storageFactory,
			streamID:       s.id,
			mpegtsWriter:   s.mpegtsWriter,
			id:             s.nextSegmentID,
			startNTP:       nextNTP,
			startDTS:       nextDTS,
		}
		err = seg.initialize()
		if err != nil {
			return err
		}
		s.nextSegment = seg

		s.mpegtsSwitchableWriter.w = seg.bw
	} else {
		seg := &muxerSegmentFMP4{
			prefix:             s.prefix,
			storageFactory:     s.storageFactory,
			streamID:           s.id,
			id:                 s.nextSegmentID,
			startNTP:           nextNTP,
			startDTS:           nextDTS,
			fromForcedRotation: force,
		}
		err = seg.initialize()
		if err != nil {
			return err
		}
		s.nextSegment = seg

		s.nextPart = &muxerPart{
			segmentMaxSize: s.segmentMaxSize,
			streamID:       s.id,
			streamTracks:   s.tracks,
			segment:        seg,
			startDTS:       seg.startDTS,
			prefix:         seg.prefix,
			id:             s.nextPartID,
			storage:        seg.storage.NewPart(),
		}
		s.nextPart.initialize()
	}

	if s.isLeading {
		targetDuration := targetDuration(s.segments)
		if s.targetDuration == 0 {
			s.targetDuration = targetDuration
		} else if targetDuration > s.targetDuration {
			s.onEncodeError(fmt.Errorf(
				"segment duration changed from %ds to %ds - this will cause an error in iOS clients",
				s.targetDuration, targetDuration))
			s.targetDuration = targetDuration
		}
	}

	return nil
}
