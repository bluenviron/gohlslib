package gohlslib

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/bluenviron/gohlslib/pkg/storage"
)

type muxerSegmentFMP4 struct {
	lowLatency          bool
	id                  uint64
	startTime           time.Time
	startDTS            time.Duration
	segmentMaxSize      uint64
	videoTrack          *Track
	audioTrack          *Track
	audioTrackTimeScale uint32
	genPartID           func() uint64
	onPartFinalized     func(*muxerPart)

	name          string
	storage       storage.File
	size          uint64
	parts         []*muxerPart
	currentPart   *muxerPart
	finalDuration time.Duration
}

func newMuxerSegmentFMP4(
	lowLatency bool,
	id uint64,
	startTime time.Time,
	startDTS time.Duration,
	segmentMaxSize uint64,
	videoTrack *Track,
	audioTrack *Track,
	audioTrackTimeScale uint32,
	factory storage.Factory,
	genPartID func() uint64,
	onPartFinalized func(*muxerPart),
) (*muxerSegmentFMP4, error) {
	s := &muxerSegmentFMP4{
		lowLatency:          lowLatency,
		id:                  id,
		startTime:           startTime,
		startDTS:            startDTS,
		segmentMaxSize:      segmentMaxSize,
		videoTrack:          videoTrack,
		audioTrack:          audioTrack,
		audioTrackTimeScale: audioTrackTimeScale,
		genPartID:           genPartID,
		onPartFinalized:     onPartFinalized,
		name:                "seg" + strconv.FormatUint(id, 10),
	}

	var err error
	s.storage, err = factory.NewFile(s.name + ".mp4")
	if err != nil {
		return nil, err
	}

	s.currentPart = newMuxerPart(
		s.videoTrack,
		s.audioTrack,
		s.audioTrackTimeScale,
		s.genPartID(),
		s.storage.NewPart(),
	)

	return s, nil
}

func (s *muxerSegmentFMP4) close() {
	s.storage.Remove()
}

func (s *muxerSegmentFMP4) getName() string {
	return s.name
}

func (s *muxerSegmentFMP4) getDuration() time.Duration {
	return s.finalDuration
}

func (s *muxerSegmentFMP4) getSize() uint64 {
	return s.storage.Size()
}

func (s *muxerSegmentFMP4) reader() (io.ReadCloser, error) {
	return s.storage.Reader()
}

func (s *muxerSegmentFMP4) finalize(
	nextVideoSampleDTS time.Duration,
) error {
	if s.currentPart.videoSamples != nil || s.currentPart.audioSamples != nil {
		err := s.currentPart.finalize()
		if err != nil {
			return err
		}

		s.onPartFinalized(s.currentPart)
		s.parts = append(s.parts, s.currentPart)
	}
	s.currentPart = nil

	s.storage.Finalize()

	if s.videoTrack != nil {
		s.finalDuration = nextVideoSampleDTS - s.startDTS
	} else {
		s.finalDuration = 0
		for _, pa := range s.parts {
			s.finalDuration += pa.finalDuration
		}
	}

	return nil
}

func (s *muxerSegmentFMP4) writeH264(sample *augmentedVideoSample, adjustedPartDuration time.Duration) error {
	size := uint64(len(sample.Payload))
	if (s.size + size) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	s.size += size

	s.currentPart.writeH264(sample)

	// switch part
	if s.lowLatency &&
		s.currentPart.duration() >= adjustedPartDuration {
		err := s.currentPart.finalize()
		if err != nil {
			return err
		}

		s.parts = append(s.parts, s.currentPart)
		s.onPartFinalized(s.currentPart)

		s.currentPart = newMuxerPart(
			s.videoTrack,
			s.audioTrack,
			s.audioTrackTimeScale,
			s.genPartID(),
			s.storage.NewPart(),
		)
	}

	return nil
}

func (s *muxerSegmentFMP4) writeAudio(sample *augmentedAudioSample, adjustedPartDuration time.Duration) error {
	size := uint64(len(sample.Payload))
	if (s.size + size) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	s.size += size

	s.currentPart.writeAudio(sample)

	// switch part
	if s.lowLatency && s.videoTrack == nil &&
		s.currentPart.duration() >= adjustedPartDuration {
		err := s.currentPart.finalize()
		if err != nil {
			return err
		}

		s.parts = append(s.parts, s.currentPart)
		s.onPartFinalized(s.currentPart)

		s.currentPart = newMuxerPart(
			s.videoTrack,
			s.audioTrack,
			s.audioTrackTimeScale,
			s.genPartID(),
			s.storage.NewPart(),
		)
	}

	return nil
}
