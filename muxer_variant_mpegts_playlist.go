package gohlslib

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"

	"github.com/bluenviron/gohlslib/pkg/playlist"
)

type muxerVariantMPEGTSPlaylist struct {
	segmentCount int

	mutex              sync.Mutex
	cond               *sync.Cond
	closed             bool
	segments           []*muxerVariantMPEGTSSegment
	segmentByName      map[string]*muxerVariantMPEGTSSegment
	segmentDeleteCount int
}

func newMuxerVariantMPEGTSPlaylist(segmentCount int) *muxerVariantMPEGTSPlaylist {
	p := &muxerVariantMPEGTSPlaylist{
		segmentCount:  segmentCount,
		segmentByName: make(map[string]*muxerVariantMPEGTSSegment),
	}
	p.cond = sync.NewCond(&p.mutex)

	return p
}

func (p *muxerVariantMPEGTSPlaylist) close() {
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

func (p *muxerVariantMPEGTSPlaylist) file(name string) *MuxerFileResponse {
	switch {
	case name == "stream.m3u8":
		return p.playlistReader()

	case strings.HasSuffix(name, ".ts"):
		return p.segmentReader(name)

	default:
		return &MuxerFileResponse{Status: http.StatusNotFound}
	}
}

func (p *muxerVariantMPEGTSPlaylist) playlistReader() *MuxerFileResponse {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.closed && len(p.segments) == 0 {
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
		Body: io.NopCloser(bytes.NewReader(p.generatePlaylist())),
	}
}

func targetDurationMPEGTS(segments []*muxerVariantMPEGTSSegment) int {
	ret := int(0)

	// EXTINF, when rounded to the nearest integer, must be <= EXT-X-TARGETDURATION
	for _, s := range segments {
		v2 := int(math.Round(s.duration().Seconds()))
		if v2 > ret {
			ret = v2
		}
	}

	return ret
}

func (p *muxerVariantMPEGTSPlaylist) generatePlaylist() []byte {
	pl := &playlist.Media{
		Version: 3,
		AllowCache: func() *bool {
			v := false
			return &v
		}(),
		TargetDuration: targetDurationMPEGTS(p.segments),
		MediaSequence:  p.segmentDeleteCount,
	}

	for _, s := range p.segments {
		pl.Segments = append(pl.Segments, &playlist.MediaSegment{
			DateTime: &s.startTime,
			Duration: s.duration(),
			URI:      s.name + ".ts",
		})
	}

	byts, _ := pl.Marshal()

	return byts
}

func (p *muxerVariantMPEGTSPlaylist) segmentReader(fname string) *MuxerFileResponse {
	base := strings.TrimSuffix(fname, ".ts")

	p.mutex.Lock()
	f, ok := p.segmentByName[base]
	p.mutex.Unlock()

	if !ok {
		return &MuxerFileResponse{Status: http.StatusNotFound}
	}

	r, err := f.reader()
	if err != nil {
		return &MuxerFileResponse{Status: http.StatusInternalServerError}
	}

	return &MuxerFileResponse{
		Status: http.StatusOK,
		Header: map[string]string{
			"Content-Type": "video/MP2T",
		},
		Body: r,
	}
}

func (p *muxerVariantMPEGTSPlaylist) pushSegment(t *muxerVariantMPEGTSSegment) {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		p.segmentByName[t.name] = t
		p.segments = append(p.segments, t)

		if len(p.segments) > p.segmentCount {
			toDelete := p.segments[0]

			toDelete.close()
			delete(p.segmentByName, toDelete.name)

			p.segments = p.segments[1:]
			p.segmentDeleteCount++
		}
	}()

	p.cond.Broadcast()
}
