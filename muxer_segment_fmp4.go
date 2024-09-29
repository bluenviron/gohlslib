package gohlslib

import (
	"fmt"
	"io"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/storage"
)

type muxerSegmentFMP4 struct {
	variant            MuxerVariant
	segmentMaxSize     uint64
	prefix             string
	nextPartID         uint64
	storageFactory     storage.Factory
	stream             *muxerStream
	id                 uint64
	startNTP           time.Time
	startDTS           time.Duration
	fromForcedRotation bool

	path    string
	storage storage.File
	size    uint64
	parts   []*muxerPart
	endDTS  time.Duration
}

func (s *muxerSegmentFMP4) initialize() error {
	s.path = segmentPath(s.prefix, s.stream.id, s.id, true)

	var err error
	s.storage, err = s.storageFactory.NewFile(s.path)
	if err != nil {
		return err
	}

	s.stream.nextPart = &muxerPart{
		stream:   s.stream,
		segment:  s,
		startDTS: s.startDTS,
		prefix:   s.prefix,
		id:       s.nextPartID,
		storage:  s.storage.NewPart(),
	}
	s.stream.nextPart.initialize()

	return nil
}

func (s *muxerSegmentFMP4) close() {
	s.storage.Remove()
}

func (s *muxerSegmentFMP4) getPath() string {
	return s.path
}

func (s *muxerSegmentFMP4) getDuration() time.Duration {
	return s.endDTS - s.startDTS
}

func (s *muxerSegmentFMP4) getSize() uint64 {
	return s.storage.Size()
}

func (s *muxerSegmentFMP4) isFromForcedRotation() bool {
	return s.fromForcedRotation
}

func (s *muxerSegmentFMP4) reader() (io.ReadCloser, error) {
	return s.storage.Reader()
}

func (s *muxerSegmentFMP4) finalize(nextDTS time.Duration) error {
	s.storage.Finalize()

	s.endDTS = nextDTS

	return nil
}

func (s *muxerSegmentFMP4) writeSample(
	track *muxerTrack,
	sample *fmp4AugmentedSample,
) error {
	size := uint64(len(sample.Payload))
	if (s.size + size) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	s.size += size

	s.stream.nextPart.writeSample(track, sample)

	return nil
}
