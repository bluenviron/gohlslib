package hls

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"

	"github.com/bluenviron/gohlslib/pkg/playlist"
)

type muxerVariantFMP4SegmentOrGap interface {
	getRenderedDuration() time.Duration
}

type muxerVariantFMP4Gap struct {
	renderedDuration time.Duration
}

func (g muxerVariantFMP4Gap) getRenderedDuration() time.Duration {
	return g.renderedDuration
}

func targetDurationFMP4(segments []muxerVariantFMP4SegmentOrGap) int {
	ret := int(0)

	// EXTINF, when rounded to the nearest integer, must be <= EXT-X-TARGETDURATION
	for _, sog := range segments {
		v := int(math.Round(sog.getRenderedDuration().Seconds()))
		if v > ret {
			ret = v
		}
	}

	return ret
}

func partTargetDuration(
	segments []muxerVariantFMP4SegmentOrGap,
	nextSegmentParts []*muxerVariantFMP4Part,
) time.Duration {
	var ret time.Duration

	for _, sog := range segments {
		seg, ok := sog.(*muxerVariantFMP4Segment)
		if !ok {
			continue
		}

		for _, part := range seg.parts {
			if part.renderedDuration > ret {
				ret = part.renderedDuration
			}
		}
	}

	for _, part := range nextSegmentParts {
		if part.renderedDuration > ret {
			ret = part.renderedDuration
		}
	}

	return ret
}

type muxerVariantFMP4Playlist struct {
	lowLatency   bool
	segmentCount int
	videoTrack   format.Format
	audioTrack   format.Format

	mutex              sync.Mutex
	cond               *sync.Cond
	closed             bool
	segments           []muxerVariantFMP4SegmentOrGap
	segmentsByName     map[string]*muxerVariantFMP4Segment
	segmentDeleteCount int
	partsByName        map[string]*muxerVariantFMP4Part
	nextSegmentID      uint64
	nextSegmentParts   []*muxerVariantFMP4Part
	nextPartID         uint64
}

func newMuxerVariantFMP4Playlist(
	lowLatency bool,
	segmentCount int,
	videoTrack format.Format,
	audioTrack format.Format,
) *muxerVariantFMP4Playlist {
	p := &muxerVariantFMP4Playlist{
		lowLatency:     lowLatency,
		segmentCount:   segmentCount,
		videoTrack:     videoTrack,
		audioTrack:     audioTrack,
		segmentsByName: make(map[string]*muxerVariantFMP4Segment),
		partsByName:    make(map[string]*muxerVariantFMP4Part),
	}
	p.cond = sync.NewCond(&p.mutex)

	return p
}

func (p *muxerVariantFMP4Playlist) close() {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		p.closed = true
	}()

	p.cond.Broadcast()

	for _, segment := range p.segments {
		if segment2, ok := segment.(*muxerVariantFMP4Segment); ok {
			segment2.close()
		}
	}
}

func (p *muxerVariantFMP4Playlist) hasContent() bool {
	if p.lowLatency {
		return len(p.segments) >= 1
	}
	return len(p.segments) >= 2
}

func (p *muxerVariantFMP4Playlist) hasPart(segmentID uint64, partID uint64) bool {
	if !p.hasContent() {
		return false
	}

	for _, sop := range p.segments {
		seg, ok := sop.(*muxerVariantFMP4Segment)
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

func (p *muxerVariantFMP4Playlist) file(name string, msn string, part string, skip string) *MuxerFileResponse {
	switch {
	case name == "stream.m3u8":
		return p.playlistReader(msn, part, skip)

	case strings.HasSuffix(name, ".mp4"):
		return p.segmentReader(name)

	default:
		return &MuxerFileResponse{Status: http.StatusNotFound}
	}
}

func (p *muxerVariantFMP4Playlist) playlistReader(msn string, part string, skip string) *MuxerFileResponse {
	isDeltaUpdate := false

	if p.lowLatency {
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

func (p *muxerVariantFMP4Playlist) generatePlaylist(isDeltaUpdate bool) []byte {
	targetDuration := targetDurationFMP4(p.segments)
	skipBoundary := time.Duration(targetDuration) * 6 * time.Second

	pl := &playlist.Media{
		Version:        9,
		TargetDuration: targetDuration,
		MediaSequence:  p.segmentDeleteCount,
	}

	if p.lowLatency {
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
			curDuration += segment.getRenderedDuration()
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
		case *muxerVariantFMP4Segment:
			plse := &playlist.MediaSegment{
				Duration: seg.renderedDuration,
				URI:      seg.name + ".mp4",
			}

			if (len(p.segments) - i) <= 2 {
				plse.DateTime = &seg.startTime
			}

			if p.lowLatency && (len(p.segments)-i) <= 2 {
				for _, part := range seg.parts {
					plse.Parts = append(plse.Parts, &playlist.MediaPart{
						Duration:    part.renderedDuration,
						URI:         part.name() + ".mp4",
						Independent: part.isIndependent,
					})
				}
			}

			pl.Segments = append(pl.Segments, plse)

		case *muxerVariantFMP4Gap:
			pl.Segments = append(pl.Segments, &playlist.MediaSegment{
				Gap:      true,
				Duration: seg.renderedDuration,
				URI:      "gap.mp4",
			})
		}
	}

	if p.lowLatency {
		for _, part := range p.nextSegmentParts {
			pl.Parts = append(pl.Parts, &playlist.MediaPart{
				Duration:    part.renderedDuration,
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

func (p *muxerVariantFMP4Playlist) segmentReader(fname string) *MuxerFileResponse {
	switch {
	case strings.HasPrefix(fname, "seg"):
		base := strings.TrimSuffix(fname, ".mp4")

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
				"Content-Type": "video/mp4",
			},
			Body: r,
		}

	case strings.HasPrefix(fname, "part"):
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

func (p *muxerVariantFMP4Playlist) onSegmentFinalized(segment *muxerVariantFMP4Segment) {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		// add initial gaps, required by iOS LL-HLS
		if p.lowLatency && len(p.segments) == 0 {
			for i := 0; i < 7; i++ {
				p.segments = append(p.segments, &muxerVariantFMP4Gap{
					renderedDuration: segment.renderedDuration,
				})
			}
		}

		p.segmentsByName[segment.name] = segment
		p.segments = append(p.segments, segment)
		p.nextSegmentID = segment.id + 1
		p.nextSegmentParts = p.nextSegmentParts[:0]

		if len(p.segments) > p.segmentCount {
			toDelete := p.segments[0]

			if toDeleteSeg, ok := toDelete.(*muxerVariantFMP4Segment); ok {
				for _, part := range toDeleteSeg.parts {
					delete(p.partsByName, part.name())
				}

				toDeleteSeg.close()
				delete(p.segmentsByName, toDeleteSeg.name)
			}

			p.segments = p.segments[1:]
			p.segmentDeleteCount++
		}
	}()

	p.cond.Broadcast()
}

func (p *muxerVariantFMP4Playlist) onPartFinalized(part *muxerVariantFMP4Part) {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		p.partsByName[part.name()] = part
		p.nextSegmentParts = append(p.nextSegmentParts, part)
		p.nextPartID = part.id + 1
	}()

	p.cond.Broadcast()
}
