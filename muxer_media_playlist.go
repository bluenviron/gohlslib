package gohlslib

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gohlslib/pkg/playlist"
)

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

	return ret
}

type muxerMediaPlaylist struct {
	variant      MuxerVariant
	segmentCount int

	mutex              sync.Mutex
	cond               *sync.Cond
	closed             bool
	segments           []muxerSegment
	segmentsByName     map[string]muxerSegment
	segmentDeleteCount int
	partsByName        map[string]*muxerPart
	nextSegmentID      uint64
	nextSegmentParts   []*muxerPart
	nextPartID         uint64
}

func newMuxerMediaPlaylist(
	variant MuxerVariant,
	segmentCount int,
) *muxerMediaPlaylist {
	p := &muxerMediaPlaylist{
		variant:        variant,
		segmentCount:   segmentCount,
		segmentsByName: make(map[string]muxerSegment),
		partsByName:    make(map[string]*muxerPart),
	}
	p.cond = sync.NewCond(&p.mutex)

	return p
}

func (p *muxerMediaPlaylist) close() {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		p.closed = true
	}()

	p.cond.Broadcast()

	for _, segment := range p.segments {
		segment.close()
	}
}

func (p *muxerMediaPlaylist) hasContent() bool {
	if p.variant == MuxerVariantFMP4 {
		return len(p.segments) >= 2
	}
	return len(p.segments) >= 1
}

func (p *muxerMediaPlaylist) hasPart(segmentID uint64, partID uint64) bool {
	if !p.hasContent() {
		return false
	}

	for _, sop := range p.segments {
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

	if segmentID != p.nextSegmentID {
		return false
	}

	if partID >= uint64(len(p.nextSegmentParts)) {
		return false
	}

	return true
}

func (p *muxerMediaPlaylist) file(name string, msn string, part string, skip string) *MuxerFileResponse {
	switch {
	case name == "stream.m3u8":
		return p.playlistReader(msn, part, skip)

	case (p.variant != MuxerVariantMPEGTS && strings.HasSuffix(name, ".mp4")) ||
		(p.variant == MuxerVariantMPEGTS && strings.HasSuffix(name, ".ts")):
		return p.segmentReader(name)

	default:
		return &MuxerFileResponse{Status: http.StatusNotFound}
	}
}

func (p *muxerMediaPlaylist) playlistReader(msn string, part string, skip string) *MuxerFileResponse {
	isDeltaUpdate := false

	if p.variant == MuxerVariantLowLatency {
		isDeltaUpdate = skip == "YES" || skip == "v2"

		var msnint uint64
		if msn != "" {
			var err error
			msnint, err = strconv.ParseUint(msn, 10, 64)
			if err != nil {
				return &MuxerFileResponse{Status: http.StatusBadRequest}
			}
		}

		var partint uint64
		if part != "" {
			var err error
			partint, err = strconv.ParseUint(part, 10, 64)
			if err != nil {
				return &MuxerFileResponse{Status: http.StatusBadRequest}
			}
		}

		if msn != "" {
			p.mutex.Lock()
			defer p.mutex.Unlock()

			// If the _HLS_msn is greater than the Media Sequence Number of the last
			// Media Segment in the current Playlist plus two, or if the _HLS_part
			// exceeds the last Partial Segment in the current Playlist by the
			// Advance Part Limit, then the server SHOULD immediately return Bad
			// Request, such as HTTP 400.
			if msnint > (p.nextSegmentID + 1) {
				return &MuxerFileResponse{Status: http.StatusBadRequest}
			}

			for !p.closed && !p.hasPart(msnint, partint) {
				p.cond.Wait()
			}

			if p.closed {
				return &MuxerFileResponse{Status: http.StatusNotFound}
			}

			return &MuxerFileResponse{
				Status: http.StatusOK,
				Header: map[string]string{
					"Content-Type": `application/x-mpegURL`,
				},
				Body: io.NopCloser(bytes.NewReader(p.generatePlaylist(isDeltaUpdate))),
			}
		}

		// part without msn is not supported.
		if part != "" {
			return &MuxerFileResponse{Status: http.StatusBadRequest}
		}
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	for !p.closed && !p.hasContent() {
		p.cond.Wait()
	}

	if p.closed {
		return &MuxerFileResponse{Status: http.StatusNotFound}
	}

	return &MuxerFileResponse{
		Status: http.StatusOK,
		Header: map[string]string{
			"Content-Type": `application/x-mpegURL`,
		},
		Body: io.NopCloser(bytes.NewReader(p.generatePlaylist(isDeltaUpdate))),
	}
}

func (p *muxerMediaPlaylist) generatePlaylist(isDeltaUpdate bool) []byte {
	if p.variant == MuxerVariantMPEGTS {
		return p.generatePlaylistMPEGTS()
	}
	return p.generatePlaylistFMP4(isDeltaUpdate)
}

func (p *muxerMediaPlaylist) generatePlaylistMPEGTS() []byte {
	pl := &playlist.Media{
		Version: 3,
		AllowCache: func() *bool {
			v := false
			return &v
		}(),
		TargetDuration: targetDuration(p.segments),
		MediaSequence:  p.segmentDeleteCount,
	}

	for _, s := range p.segments {
		if seg, ok := s.(*muxerSegmentMPEGTS); ok {
			pl.Segments = append(pl.Segments, &playlist.MediaSegment{
				DateTime: &seg.startTime,
				Duration: seg.getDuration(),
				URI:      seg.name + ".ts",
			})
		}
	}

	byts, _ := pl.Marshal()
	return byts
}

func (p *muxerMediaPlaylist) generatePlaylistFMP4(isDeltaUpdate bool) []byte {
	targetDuration := targetDuration(p.segments)
	skipBoundary := time.Duration(targetDuration) * 6 * time.Second

	pl := &playlist.Media{
		Version:        9,
		TargetDuration: targetDuration,
		MediaSequence:  p.segmentDeleteCount,
	}

	if p.variant == MuxerVariantLowLatency {
		partTarget := partTargetDuration(p.segments, p.nextSegmentParts)
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
			URI: "init.mp4",
		}
	} else {
		var curDuration time.Duration
		shown := 0
		for _, segment := range p.segments {
			curDuration += segment.getDuration()
			if curDuration >= skipBoundary {
				break
			}
			shown++
		}
		skipped = len(p.segments) - shown

		pl.Skip = &playlist.MediaSkip{
			SkippedSegments: skipped,
		}
	}

	for i, sog := range p.segments {
		if i < skipped {
			continue
		}

		switch seg := sog.(type) {
		case *muxerSegmentFMP4:
			plse := &playlist.MediaSegment{
				Duration: seg.finalDuration,
				URI:      seg.name + ".mp4",
			}

			if (len(p.segments) - i) <= 2 {
				plse.DateTime = &seg.startTime
			}

			if p.variant == MuxerVariantLowLatency && (len(p.segments)-i) <= 2 {
				for _, part := range seg.parts {
					plse.Parts = append(plse.Parts, &playlist.MediaPart{
						Duration:    part.finalDuration,
						URI:         part.name() + ".mp4",
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

	if p.variant == MuxerVariantLowLatency {
		for _, part := range p.nextSegmentParts {
			pl.Parts = append(pl.Parts, &playlist.MediaPart{
				Duration:    part.finalDuration,
				URI:         part.name() + ".mp4",
				Independent: part.isIndependent,
			})
		}

		// preload hint must always be present
		// otherwise hls.js goes into a loop
		pl.PreloadHint = &playlist.MediaPreloadHint{
			URI: fmp4PartName(p.nextPartID) + ".mp4",
		}
	}

	byts, _ := pl.Marshal()
	return byts
}

func (p *muxerMediaPlaylist) segmentReader(fname string) *MuxerFileResponse {
	switch {
	case strings.HasPrefix(fname, "seg"):
		base := strings.TrimSuffix(strings.TrimSuffix(fname, ".mp4"), ".ts")

		p.mutex.Lock()
		segment, ok := p.segmentsByName[base]
		p.mutex.Unlock()

		if !ok {
			return &MuxerFileResponse{Status: http.StatusNotFound}
		}

		r, err := segment.reader()
		if err != nil {
			return &MuxerFileResponse{Status: http.StatusInternalServerError}
		}

		return &MuxerFileResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": func() string {
					if p.variant == MuxerVariantMPEGTS {
						return "video/MP2T"
					}
					return "video/mp4"
				}(),
			},
			Body: r,
		}

	case p.variant == MuxerVariantLowLatency && strings.HasPrefix(fname, "part"):
		base := strings.TrimSuffix(fname, ".mp4")

		p.mutex.Lock()
		part, ok := p.partsByName[base]
		nextPartID := p.nextPartID
		p.mutex.Unlock()

		if ok {
			r, err := part.reader()
			if err != nil {
				return &MuxerFileResponse{Status: http.StatusInternalServerError}
			}

			return &MuxerFileResponse{
				Status: http.StatusOK,
				Header: map[string]string{
					"Content-Type": "video/mp4",
				},
				Body: r,
			}
		}

		// EXT-X-PRELOAD-HINT support
		nextPartName := fmp4PartName(p.nextPartID)
		if base == nextPartName {
			p.mutex.Lock()
			defer p.mutex.Unlock()

			for {
				if p.closed {
					break
				}

				if p.nextPartID > nextPartID {
					break
				}

				p.cond.Wait()
			}

			if p.closed {
				return &MuxerFileResponse{Status: http.StatusNotFound}
			}

			r, err := p.partsByName[nextPartName].reader()
			if err != nil {
				return &MuxerFileResponse{Status: http.StatusInternalServerError}
			}

			return &MuxerFileResponse{
				Status: http.StatusOK,
				Header: map[string]string{
					"Content-Type": "video/mp4",
				},
				Body: r,
			}
		}

		return &MuxerFileResponse{Status: http.StatusNotFound}

	default:
		return &MuxerFileResponse{Status: http.StatusNotFound}
	}
}

func (p *muxerMediaPlaylist) bandwidth() (int, int) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(p.segments) == 0 {
		return 0, 0
	}

	var maxBandwidth uint64
	var sizes uint64
	var durations time.Duration

	for _, seg := range p.segments {
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

func (p *muxerMediaPlaylist) onSegmentFinalized(segment muxerSegment) {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		// add initial gaps, required by iOS LL-HLS
		if p.variant == MuxerVariantLowLatency && len(p.segments) == 0 {
			for i := 0; i < 7; i++ {
				p.segments = append(p.segments, &muxerGap{
					duration: segment.getDuration(),
				})
			}
		}

		p.segmentsByName[segment.getName()] = segment
		p.segments = append(p.segments, segment)

		if seg, ok := segment.(*muxerSegmentFMP4); ok {
			p.nextSegmentID = seg.id + 1
		}

		p.nextSegmentParts = p.nextSegmentParts[:0]

		if len(p.segments) > p.segmentCount {
			toDelete := p.segments[0]

			if toDeleteSeg, ok := toDelete.(*muxerSegmentFMP4); ok {
				for _, part := range toDeleteSeg.parts {
					delete(p.partsByName, part.name())
				}
			}

			toDelete.close()
			delete(p.segmentsByName, toDelete.getName())

			p.segments = p.segments[1:]
			p.segmentDeleteCount++
		}
	}()

	p.cond.Broadcast()
}

func (p *muxerMediaPlaylist) onPartFinalized(part *muxerPart) {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		p.partsByName[part.name()] = part
		p.nextSegmentParts = append(p.nextSegmentParts, part)
		p.nextPartID = part.id + 1
	}()

	p.cond.Broadcast()
}
