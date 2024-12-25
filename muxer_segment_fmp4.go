package gohlslib

import (
	"io"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/storage"
)

type muxerSegmentFMP4 struct {
	prefix             string
	storageFactory     storage.Factory
	streamID           string
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
	s.path = segmentPath(s.prefix, s.streamID, s.id, true)

	var err error
	s.storage, err = s.storageFactory.NewFile(s.path)
	if err != nil {
		return err
	}

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
