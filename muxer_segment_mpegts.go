package gohlslib

import (
	"bufio"
	"fmt"
	"io"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/storage"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
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

	storage     storage.File
	storagePart storage.Part
	bw          *bufio.Writer
	size        uint64
	path        string
	endDTS      time.Duration // available after finalize()
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

	err := s.mpegtsWriter.WriteH264(
		track.mpegtsTrack,
		pts,
		dts,
		au,
	)
	if err != nil {
		return err
	}

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

	tolerance := durationToTimestamp(mpegtsAACPTSDriftTolerance, track.ClockRate)

	// recompute timestamp from scratch.
	// iOS+MPEG-TS+AAC requires a precise timestamp that might get lost during timestamp conversion.
	// also reset in case of drifts.
	if !track.mpegtsAACPTSInitialized ||
		track.mpegtsAACPTS > (pts+tolerance) ||
		track.mpegtsAACPTS < (pts-tolerance) {
		track.mpegtsAACPTS = pts
		track.mpegtsAACPTSInitialized = true
	}

	err := s.mpegtsWriter.WriteMPEG4Audio(
		track.mpegtsTrack,
		multiplyAndDivide(track.mpegtsAACPTS, 90000, int64(track.ClockRate)),
		aus,
	)
	if err != nil {
		return err
	}

	track.mpegtsAACPTS += mpeg4audio.SamplesPerAccessUnit * int64(len(aus))

	return nil
}

func (s *muxerSegmentMPEGTS) writeKLV(
	track *muxerTrack,
	pts int64,
	data []byte,
) error {
	size := uint64(len(data))
	if (s.size + size) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	s.size += size

	return s.mpegtsWriter.WriteKLV(
		track.mpegtsTrack,
		pts,
		data,
	)
}
