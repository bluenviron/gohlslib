package gohlslib

import (
	"io"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/storage"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
)

type muxerPart struct {
	stream   *muxerStream
	segment  *muxerSegmentFMP4
	startDTS time.Duration
	prefix   string
	id       uint64
	storage  storage.Part

	path          string
	isIndependent bool
	endDTS        time.Duration
}

func (p *muxerPart) initialize() {
	p.path = partPath(p.prefix, p.stream.id, p.id)
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

	for i, track := range p.stream.tracks {
		if track.fmp4Samples != nil {
			part.Tracks = append(part.Tracks, &fmp4.PartTrack{
				ID:       1 + i,
				BaseTime: durationGoToMp4(track.fmp4StartDTS, track.fmp4TimeScale),
				Samples:  track.fmp4Samples,
			})

			track.fmp4StartDTSFilled = false
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

func (p *muxerPart) writeSample(track *muxerTrack, sample *fmp4AugmentedSample) {
	if !track.fmp4StartDTSFilled {
		track.fmp4StartDTSFilled = true
		track.fmp4StartDTS = sample.dts
	}

	if (track.isLeading || len(track.stream.tracks) == 1) && !sample.IsNonSyncSample {
		p.isIndependent = true
	}

	track.fmp4Samples = append(track.fmp4Samples, &sample.PartSample)
}
