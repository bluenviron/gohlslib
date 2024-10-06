package gohlslib

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist"
)

const (
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

type muxerServer struct {
	muxer *Muxer // TODO: remove

	pathHandlers map[string]http.HandlerFunc
}

func (s *muxerServer) initialize() {
	s.pathHandlers = make(map[string]http.HandlerFunc)

	s.pathHandlers["index.m3u8"] = s.handleMultivariantPlaylist
}

func (s *muxerServer) handle(w http.ResponseWriter, r *http.Request) {
	path := filepath.Base(r.URL.Path)

	s.muxer.mutex.Lock()
	handler, ok := s.pathHandlers[path]
	s.muxer.mutex.Unlock()
	if ok {
		handler(w, r)
		return
	}
}

func (s *muxerServer) handleMultivariantPlaylist(w http.ResponseWriter, r *http.Request) {
	buf := func() []byte {
		s.muxer.mutex.Lock()
		defer s.muxer.mutex.Unlock()

		for {
			if s.muxer.closed {
				return nil
			}

			if s.muxer.streams[0].hasContent() {
				break
			}

			s.muxer.cond.Wait()
		}

		buf, err := s.generateMultivariantPlaylist(r.URL.RawQuery)
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

func (s *muxerServer) generateMultivariantPlaylist(rawQuery string) ([]byte, error) {
	// TODO: consider segments in all streams
	maxBandwidth, averageBandwidth := bandwidth(s.muxer.streams[0].segments)

	pl := &playlist.Multivariant{
		Version: func() int {
			if s.muxer.Variant == MuxerVariantMPEGTS {
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

	for _, stream := range s.muxer.streams {
		err := stream.populateMultivariantPlaylist(pl, rawQuery)
		if err != nil {
			return nil, err
		}
	}

	return pl.Marshal()
}
