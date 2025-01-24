package gohlslib

import (
	"bufio"
	"fmt"
	"io"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/storage"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
)

type muxerSegmentMPEGTS struct {
	segmentMaxSize uint64
	prefix         string
	storageFactory storage.Factory
	streamID       string
	mpegtsWriter   *mpegts.Writer
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
	s.path = segmentPath(s.prefix, s.streamID, s.id, false)

	var err error
	s.storage, err = s.storageFactory.NewFile(s.path)
	if err != nil {
		return err
	}

	s.storagePart = s.storage.NewPart()
	s.bw = bufio.NewWriter(s.storagePart.Writer())

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
	pts int64,
	dts int64,
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

	err := s.mpegtsWriter.WriteH2642(
		track.mpegtsTrack,
		multiplyAndDivide(pts, 90000, int64(track.ClockRate)),
		multiplyAndDivide(dts, 90000, int64(track.ClockRate)),
		au,
	)
	if err != nil {
		return err
	}

	s.endDTS = timestampToDuration(dts, track.ClockRate)

	return nil
}

func (s *muxerSegmentMPEGTS) writeMPEG4Audio(
	track *muxerTrack,
	pts int64,
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

	err := s.mpegtsWriter.WriteMPEG4Audio(
		track.mpegtsTrack,
		multiplyAndDivide(pts, 90000, int64(track.ClockRate)),
		aus,
	)
	if err != nil {
		return err
	}

	if track.isLeading {
		s.audioAUCount++
		s.endDTS = timestampToDuration(pts, track.ClockRate)
	}

	return nil
}
