package gohlslib

import (
	"fmt"
	"io"
	"time"

	"github.com/bluenviron/gohlslib/pkg/storage"
)

type muxerSegmentFMP4 struct {
	lowLatency     bool
	id             uint64
	startNTP       time.Time
	startDTS       time.Duration
	segmentMaxSize uint64
	videoTrack     *Track
	audioTrack     *Track
	audioTimeScale uint32
	prefix         string
	forceSwitched  bool
	genPartID      func() uint64
	publishPart    func(*muxerPart)

	name        string
	storage     storage.File
	size        uint64
	parts       []*muxerPart
	currentPart *muxerPart
	endDTS      time.Duration
}

func newMuxerSegmentFMP4(
	lowLatency bool,
	id uint64,
	startNTP time.Time,
	startDTS time.Duration,
	segmentMaxSize uint64,
	videoTrack *Track,
	audioTrack *Track,
	audioTimeScale uint32,
	prefix string,
	forceSwitched bool,
	factory storage.Factory,
	genPartID func() uint64,
	publishPart func(*muxerPart),
) (*muxerSegmentFMP4, error) {
	s := &muxerSegmentFMP4{
		lowLatency:     lowLatency,
		id:             id,
		startNTP:       startNTP,
		startDTS:       startDTS,
		segmentMaxSize: segmentMaxSize,
		videoTrack:     videoTrack,
		audioTrack:     audioTrack,
		audioTimeScale: audioTimeScale,
		prefix:         prefix,
		forceSwitched:  forceSwitched,
		genPartID:      genPartID,
		publishPart:    publishPart,
		name:           segmentName(prefix, id, true),
	}

	var err error
	s.storage, err = factory.NewFile(s.name)
	if err != nil {
		return nil, err
	}

	s.currentPart = newMuxerPart(
		startDTS,
		s.videoTrack,
		s.audioTrack,
		s.audioTimeScale,
		prefix,
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
	return s.endDTS - s.startDTS
}

func (s *muxerSegmentFMP4) getSize() uint64 {
	return s.storage.Size()
}

func (s *muxerSegmentFMP4) isForceSwitched() bool {
	return s.forceSwitched
}

func (s *muxerSegmentFMP4) reader() (io.ReadCloser, error) {
	return s.storage.Reader()
}

func (s *muxerSegmentFMP4) finalize(nextDTS time.Duration) error {
	if s.currentPart.videoSamples != nil || s.currentPart.audioSamples != nil {
		err := s.currentPart.finalize(nextDTS)
		if err != nil {
			return err
		}

		s.publishPart(s.currentPart)
		s.parts = append(s.parts, s.currentPart)
	}
	s.currentPart = nil

	s.storage.Finalize()

	s.endDTS = nextDTS

	return nil
}

func (s *muxerSegmentFMP4) writeVideo(
	sample *augmentedVideoSample,
	nextDTS time.Duration,
	adjustedPartDuration time.Duration,
) error {
	size := uint64(len(sample.Payload))
	if (s.size + size) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	s.size += size

	s.currentPart.writeVideo(sample)

	// switch part
	if s.lowLatency &&
		s.currentPart.computeDuration(nextDTS) >= adjustedPartDuration {
		err := s.currentPart.finalize(nextDTS)
		if err != nil {
			return err
		}

		s.parts = append(s.parts, s.currentPart)
		s.publishPart(s.currentPart)

		s.currentPart = newMuxerPart(
			nextDTS,
			s.videoTrack,
			s.audioTrack,
			s.audioTimeScale,
			s.prefix,
			s.genPartID(),
			s.storage.NewPart(),
		)
	}

	return nil
}

func (s *muxerSegmentFMP4) writeAudio(
	sample *augmentedAudioSample,
	nextAudioSampleDTS time.Duration,
	adjustedPartDuration time.Duration,
) error {
	size := uint64(len(sample.Payload))
	if (s.size + size) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	s.size += size

	s.currentPart.writeAudio(sample)

	// switch part
	if s.lowLatency && s.videoTrack == nil &&
		s.currentPart.computeDuration(nextAudioSampleDTS) >= adjustedPartDuration {
		err := s.currentPart.finalize(nextAudioSampleDTS)
		if err != nil {
			return err
		}

		s.parts = append(s.parts, s.currentPart)
		s.publishPart(s.currentPart)

		s.currentPart = newMuxerPart(
			nextAudioSampleDTS,
			s.videoTrack,
			s.audioTrack,
			s.audioTimeScale,
			s.prefix,
			s.genPartID(),
			s.storage.NewPart(),
		)
	}

	return nil
}
