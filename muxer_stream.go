package gohlslib

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/codecparams"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/gohlslib/v2/pkg/playlist"
	"github.com/bluenviron/mediacommon/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4/seekablebuffer"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
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

type generateMediaPlaylistFunc func(
	isDeltaUpdate bool,
	rawQuery string,
) ([]byte, error)

type muxerStream struct {
	muxer       *Muxer // TODO: remove
	tracks      []*muxerTrack
	id          string
	isRendition bool

	generateMediaPlaylist generateMediaPlaylistFunc

	mpegtsSwitchableWriter *switchableWriter // mpegts only
	mpegtsWriter           *mpegts.Writer    // mpegts only
	segments               []muxerSegment
	nextSegment            muxerSegment
	nextPart               *muxerPart // low-latency only
	initFilePresent        bool       // fmp4 only
}

func (s *muxerStream) initialize() {
	for _, track := range s.tracks {
		track.stream = s
	}

	if s.muxer.Variant == MuxerVariantMPEGTS {
		s.generateMediaPlaylist = s.generateMediaPlaylistMPEGTS

		tracks := make([]*mpegts.Track, len(s.tracks))
		for i, track := range s.tracks {
			tracks[i] = track.mpegtsTrack
		}
		s.mpegtsSwitchableWriter = &switchableWriter{}
		s.mpegtsWriter = mpegts.NewWriter(s.mpegtsSwitchableWriter, tracks)
	} else {
		s.generateMediaPlaylist = s.generateMediaPlaylistFMP4
	}

	s.muxer.server.pathHandlers[mediaPlaylistPath(s.id)] = s.handleMediaPlaylist
}

func (s *muxerStream) close() {
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
		mv.Codecs = append(mv.Codecs, codecparams.Marshal(track.Codec))

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

	if !s.isRendition {
		mv.URI = uri
	} else {
		mv.Audio = "audio"

		r := &playlist.MultivariantRendition{
			Type:    playlist.MultivariantRenditionTypeAudio,
			GroupID: "audio",
			URI:     &uri,
		}
		pl.Renditions = append(pl.Renditions, r)
	}

	return nil
}

func (s *muxerStream) hasContent() bool {
	if s.muxer.Variant == MuxerVariantFMP4 {
		return len(s.segments) >= 2
	}
	return len(s.segments) >= 1
}

func (s *muxerStream) hasPart(segmentID uint64, partID uint64) bool {
	if segmentID == s.muxer.nextSegmentID {
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

	if s.muxer.Variant == MuxerVariantLowLatency {
		isDeltaUpdate = skip == "YES" || skip == "v2"

		msnint, partint, err := parseMSNPart(msn, part)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		switch {
		case msn != "":
			byts := func() []byte {
				s.muxer.mutex.Lock()
				defer s.muxer.mutex.Unlock()

				for {
					if s.muxer.closed {
						w.WriteHeader(http.StatusInternalServerError)
						return nil
					}

					// If the _HLS_msn is greater than the Media Sequence Number of the last
					// Media Segment in the current Playlist plus two, or if the _HLS_part
					// exceeds the last Partial Segment in the current Playlist by the
					// Advance Part Limit, then the server SHOULD immediately return Bad
					// Request, such as HTTP 400.
					if msnint > (s.muxer.nextSegmentID+1) || msnint < (s.muxer.nextSegmentID-uint64(len(s.segments)-1)) {
						w.WriteHeader(http.StatusBadRequest)
						return nil
					}

					if s.hasContent() && s.hasPart(msnint, partint) {
						break
					}

					s.muxer.cond.Wait()
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
			return

		case part != "": // part without msn is not supported.
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	byts := func() []byte {
		s.muxer.mutex.Lock()
		defer s.muxer.mutex.Unlock()

		for {
			if s.muxer.closed {
				w.WriteHeader(http.StatusInternalServerError)
				return nil
			}

			if s.hasContent() {
				break
			}

			s.muxer.cond.Wait()
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
	isDeltaUpdate bool,
	rawQuery string,
) ([]byte, error) {
	pl := &playlist.Media{
		Version:        3,
		AllowCache:     boolPtr(false),
		TargetDuration: targetDuration(s.segments),
		MediaSequence:  s.muxer.segmentDeleteCount,
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
	targetDuration := targetDuration(s.segments)
	skipBoundary := time.Duration(targetDuration) * 6 * time.Second
	rawQuery = filterOutHLSParams(rawQuery)

	pl := &playlist.Media{
		Version:        10,
		TargetDuration: targetDuration,
		MediaSequence:  s.muxer.segmentDeleteCount,
	}

	if s.muxer.Variant == MuxerVariantLowLatency {
		partTarget := partTargetDuration(s.muxer.streams)
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
		uri := initFilePath(s.muxer.prefix, s.id)
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

			if s.muxer.Variant == MuxerVariantLowLatency && (len(s.segments)-i) <= 2 {
				for _, part := range seg.parts {
					u = part.path
					if rawQuery != "" {
						u += "?" + rawQuery
					}

					plse.Parts = append(plse.Parts, &playlist.MediaPart{
						Duration:    part.finalDuration,
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

	if s.muxer.Variant == MuxerVariantLowLatency {
		for _, part := range s.nextSegment.(*muxerSegmentFMP4).parts {
			u := part.path
			if rawQuery != "" {
				u += "?" + rawQuery
			}

			pl.Parts = append(pl.Parts, &playlist.MediaPart{
				Duration:    part.finalDuration,
				URI:         u,
				Independent: part.isIndependent,
			})
		}

		// preload hint must always be present
		// otherwise hls.js goes into a loop
		uri := partPath(s.muxer.prefix, s.id, s.muxer.nextPartID)
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
			Codec:     codecs.ToFMP4(track.Codec),
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

	s.muxer.server.pathHandlers[initFilePath(s.muxer.prefix, s.id)] = func(w http.ResponseWriter, _ *http.Request) {
		// allow caching but use a small period in order to
		// allow a stream to change track parameters
		w.Header().Set("Cache-Control", "max-age="+initMaxAge)
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		w.Write(initFile)
	}

	return nil
}

func (s *muxerStream) createFirstSegment(
	nextDTS time.Duration,
	nextNTP time.Time,
) error {
	if s.muxer.Variant == MuxerVariantMPEGTS {
		seg := &muxerSegmentMPEGTS{
			segmentMaxSize: s.muxer.SegmentMaxSize,
			prefix:         s.muxer.prefix,
			storageFactory: s.muxer.storageFactory,
			stream:         s,
			id:             s.muxer.nextSegmentID,
			startNTP:       nextNTP,
			startDTS:       nextDTS,
		}
		err := seg.initialize()
		if err != nil {
			return err
		}
		s.nextSegment = seg
	} else {
		seg := &muxerSegmentFMP4{
			variant:            s.muxer.Variant,
			segmentMaxSize:     s.muxer.SegmentMaxSize,
			prefix:             s.muxer.prefix,
			nextPartID:         s.muxer.nextPartID,
			storageFactory:     s.muxer.storageFactory,
			stream:             s,
			id:                 s.muxer.nextSegmentID,
			startNTP:           nextNTP,
			startDTS:           nextDTS,
			fromForcedRotation: false,
		}
		err := seg.initialize()
		if err != nil {
			return err
		}
		s.nextSegment = seg
	}

	return nil
}

func (s *muxerStream) rotateParts(nextDTS time.Duration, createNew bool) error {
	part := s.nextPart
	s.nextPart = nil

	err := part.finalize(nextDTS)
	if err != nil {
		return err
	}

	if s.muxer.Variant == MuxerVariantLowLatency {
		part.segment.parts = append(part.segment.parts, part)

		s.muxer.server.pathHandlers[part.path] = func(w http.ResponseWriter, _ *http.Request) {
			r, err := part.reader()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			defer r.Close()

			w.Header().Set("Cache-Control", "max-age="+segmentMaxAge)
			w.Header().Set("Content-Type", "video/mp4")
			w.WriteHeader(http.StatusOK)
			io.Copy(w, r)
		}

		// EXT-X-PRELOAD-HINT
		partID := s.muxer.nextPartID
		partPath := partPath(s.muxer.prefix, s.id, partID)
		s.muxer.server.pathHandlers[partPath] = func(w http.ResponseWriter, r *http.Request) {
			s.muxer.mutex.Lock()

			for !s.muxer.closed && s.muxer.nextPartID <= partID {
				s.muxer.cond.Wait()
			}

			h := s.muxer.server.pathHandlers[partPath]

			s.muxer.mutex.Unlock()

			if h != nil {
				h(w, r)
			}
		}
	}

	if createNew {
		nextPart := &muxerPart{
			stream:   s,
			segment:  part.segment,
			startDTS: nextDTS,
			prefix:   s.muxer.prefix,
			id:       s.muxer.nextPartID,
			storage:  part.segment.storage.NewPart(),
		}
		nextPart.initialize()
		s.nextPart = nextPart
	}

	return nil
}

func (s *muxerStream) rotateSegments(
	nextDTS time.Duration,
	nextNTP time.Time,
	force bool,
) error {
	segment := s.nextSegment
	s.nextSegment = nil

	err := segment.finalize(nextDTS)
	if err != nil {
		segment.close()
		return err
	}

	// add initial gaps, required by iOS LL-HLS
	if s.muxer.Variant == MuxerVariantLowLatency && len(s.segments) == 0 {
		for i := 0; i < 7; i++ {
			s.segments = append(s.segments, &muxerGap{
				duration: segment.getDuration(),
			})
		}
	}

	s.segments = append(s.segments, segment)

	s.muxer.server.pathHandlers[segment.getPath()] = func(w http.ResponseWriter, _ *http.Request) {
		r, err := segment.reader()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer r.Close()

		w.Header().Set("Cache-Control", "max-age="+segmentMaxAge)
		w.Header().Set(
			"Content-Type",
			func() string {
				if s.muxer.Variant == MuxerVariantMPEGTS {
					return "video/MP2T"
				}
				return "video/mp4"
			}(),
		)
		w.WriteHeader(http.StatusOK)
		io.Copy(w, r)
	}

	// delete old segments and parts
	if len(s.segments) > s.muxer.SegmentCount {
		toDelete := s.segments[0]

		if toDeleteSeg, ok := toDelete.(*muxerSegmentFMP4); ok {
			for _, part := range toDeleteSeg.parts {
				delete(s.muxer.server.pathHandlers, part.path)
			}
		}

		toDelete.close()
		delete(s.muxer.server.pathHandlers, toDelete.getPath())

		s.segments = s.segments[1:]

		if s == s.muxer.streams[0] {
			s.muxer.segmentDeleteCount++
		}
	}

	// regenerate init files only if missing or codec parameters have changed
	if s.muxer.Variant != MuxerVariantMPEGTS && (!s.initFilePresent || segment.isFromForcedRotation()) {
		err := s.generateAndCacheInitFile()
		if err != nil {
			return err
		}
	}

	// create next segment
	var nextSegment muxerSegment
	if s.muxer.Variant == MuxerVariantMPEGTS {
		nextSegment = &muxerSegmentMPEGTS{
			segmentMaxSize: s.muxer.SegmentMaxSize,
			prefix:         s.muxer.prefix,
			storageFactory: s.muxer.storageFactory,
			stream:         s,
			id:             s.muxer.nextSegmentID,
			startNTP:       nextNTP,
			startDTS:       nextDTS,
		}
	} else {
		nextSegment = &muxerSegmentFMP4{
			variant:            s.muxer.Variant,
			segmentMaxSize:     s.muxer.SegmentMaxSize,
			prefix:             s.muxer.prefix,
			nextPartID:         s.muxer.nextPartID,
			storageFactory:     s.muxer.storageFactory,
			stream:             s,
			id:                 s.muxer.nextSegmentID,
			startNTP:           nextNTP,
			startDTS:           nextDTS,
			fromForcedRotation: force,
		}
	}
	err = nextSegment.initialize()
	if err != nil {
		return err
	}
	s.nextSegment = nextSegment

	return nil
}
