package gohlslib

import (
	"fmt"
	"io"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/storage"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

type muxerPart struct {
	segmentMaxSize uint64
	streamID       string
	streamTracks   []*muxerTrack
	segment        *muxerSegmentFMP4
	startDTS       time.Duration
	prefix         string
	id             uint64
	storage        storage.Part

	path          string
	isIndependent bool
	endDTS        time.Duration
}

func (p *muxerPart) initialize() {
	p.path = partPath(p.prefix, p.streamID, p.id)
}

func (p *muxerPart) reader() (io.ReadCloser, error) {
	return p.storage.Reader()
}

func (p *muxerPart) getDuration() time.Duration {
	return p.endDTS - p.startDTS
}

func (p *muxerPart) finalize(endDTS time.Duration) error {
	part := fmp4.Part{
		SequenceNumber: uint32(p.id),
	}

	for i, track := range p.streamTracks {
		if track.fmp4Samples != nil {
			part.Tracks = append(part.Tracks, &fmp4.PartTrack{
				ID:       1 + i,
				BaseTime: uint64(track.fmp4StartDTS),
				Samples:  track.fmp4Samples,
			})

			track.fmp4Samples = nil
		}
	}

	err := part.Marshal(p.storage.Writer())
	if err != nil {
		return err
	}

	p.endDTS = endDTS

	return nil
}

func (p *muxerPart) writeSample(track *muxerTrack, sample *fmp4AugmentedSample) error {
	size := uint64(len(sample.Payload))
	if (p.segment.size + size) > p.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	p.segment.size += size

	if track.fmp4Samples == nil {
		track.fmp4StartDTS = sample.dts
	}

	if (track.isLeading || len(track.stream.tracks) == 1) && !sample.IsNonSyncSample {
		p.isIndependent = true
	}

	track.fmp4Samples = append(track.fmp4Samples, &sample.PartSample)

	return nil
}
