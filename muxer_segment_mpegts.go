package gohlslib

import (
	"bufio"
	"fmt"
	"io"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/storage"
)

func durationGoToMPEGTS(v time.Duration) int64 {
	return int64(v.Seconds() * 90000)
}

type muxerSegmentMPEGTS struct {
	segmentMaxSize uint64
	prefix         string
	storageFactory storage.Factory
	stream         *muxerStream
	id             uint64
	startNTP       time.Time
	startDTS       time.Duration

	storage      storage.File
	storagePart  storage.Part
	bw           *bufio.Writer
	size         uint64
	path         string
	endDTS       time.Duration
	audioAUCount int
}

func (s *muxerSegmentMPEGTS) initialize() error {
	s.path = segmentPath(s.prefix, s.stream.id, s.id, false)

	var err error
	s.storage, err = s.storageFactory.NewFile(s.path)
	if err != nil {
		return err
	}

	s.storagePart = s.storage.NewPart()
	s.bw = bufio.NewWriter(s.storagePart.Writer())

	s.stream.mpegtsSwitchableWriter.w = s.bw

	return nil
}

func (s *muxerSegmentMPEGTS) close() {
	s.storage.Remove()
}

func (s *muxerSegmentMPEGTS) getPath() string {
	return s.path
}

func (s *muxerSegmentMPEGTS) getDuration() time.Duration {
	return s.endDTS - s.startDTS
}

func (s *muxerSegmentMPEGTS) getSize() uint64 {
	return s.storage.Size()
}

func (*muxerSegmentMPEGTS) isFromForcedRotation() bool {
	return false
}

func (s *muxerSegmentMPEGTS) reader() (io.ReadCloser, error) {
	return s.storage.Reader()
}

func (s *muxerSegmentMPEGTS) finalize(endDTS time.Duration) error {
	err := s.bw.Flush()
	if err != nil {
		return err
	}

	s.bw = nil
	s.storage.Finalize()
	s.endDTS = endDTS
	return nil
}

func (s *muxerSegmentMPEGTS) writeH264(
	track *muxerTrack,
	pts time.Duration,
	dts time.Duration,
	idrPresent bool,
	au [][]byte,
) error {
	size := uint64(0)
	for _, nalu := range au {
		size += uint64(len(nalu))
	}
	if (s.size + size) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	s.size += size

	err := s.stream.mpegtsWriter.WriteH264(
		track.mpegtsTrack,
		durationGoToMPEGTS(pts),
		durationGoToMPEGTS(dts),
		idrPresent,
		au,
	)
	if err != nil {
		return err
	}

	s.endDTS = dts

	return nil
}

func (s *muxerSegmentMPEGTS) writeMPEG4Audio(
	track *muxerTrack,
	pts time.Duration,
	aus [][]byte,
) error {
	size := uint64(0)
	for _, au := range aus {
		size += uint64(len(au))
	}

	if (s.size + size) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	s.size += size

	err := s.stream.mpegtsWriter.WriteMPEG4Audio(
		track.mpegtsTrack,
		durationGoToMPEGTS(pts),
		aus,
	)
	if err != nil {
		return err
	}

	if track.isLeading {
		s.audioAUCount++
		s.endDTS = pts
	}

	return nil
}
