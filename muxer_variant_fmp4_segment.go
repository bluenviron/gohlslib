package hls

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"

	"github.com/bluenviron/gohlslib/pkg/storage"
)

type muxerVariantFMP4Segment struct {
	lowLatency      bool
	id              uint64
	startTime       time.Time
	startDTS        time.Duration
	segmentMaxSize  uint64
	videoTrack      format.Format
	audioTrack      format.Format
	genPartID       func() uint64
	onPartFinalized func(*muxerVariantFMP4Part)

	name             string
	storage          storage.Segment
	size             uint64
	parts            []*muxerVariantFMP4Part
	currentPart      *muxerVariantFMP4Part
	renderedDuration time.Duration
}

func newMuxerVariantFMP4Segment(
	lowLatency bool,
	id uint64,
	startTime time.Time,
	startDTS time.Duration,
	segmentMaxSize uint64,
	videoTrack format.Format,
	audioTrack format.Format,
	factory storage.Factory,
	genPartID func() uint64,
	onPartFinalized func(*muxerVariantFMP4Part),
) (*muxerVariantFMP4Segment, error) {
	s := &muxerVariantFMP4Segment{
		lowLatency:      lowLatency,
		id:              id,
		startTime:       startTime,
		startDTS:        startDTS,
		segmentMaxSize:  segmentMaxSize,
		videoTrack:      videoTrack,
		audioTrack:      audioTrack,
		genPartID:       genPartID,
		onPartFinalized: onPartFinalized,
		name:            "seg" + strconv.FormatUint(id, 10),
	}

	var err error
	s.storage, err = factory.NewSegment(s.name + ".mp4")
	if err != nil {
		return nil, err
	}

	s.currentPart = newMuxerVariantFMP4Part(
		s.videoTrack,
		s.audioTrack,
		s.genPartID(),
		s.storage.NewPart(),
	)

	return s, nil
}

func (s *muxerVariantFMP4Segment) close() {
	s.storage.Remove()
}

func (s *muxerVariantFMP4Segment) reader() (io.ReadCloser, error) {
	return s.storage.Reader()
}

func (s *muxerVariantFMP4Segment) getRenderedDuration() time.Duration {
	return s.renderedDuration
}

func (s *muxerVariantFMP4Segment) finalize(
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
		s.renderedDuration = nextVideoSampleDTS - s.startDTS
	} else {
		s.renderedDuration = 0
		for _, pa := range s.parts {
			s.renderedDuration += pa.renderedDuration
		}
	}

	return nil
}

func (s *muxerVariantFMP4Segment) writeH264(sample *augmentedVideoSample, adjustedPartDuration time.Duration) error {
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

		s.currentPart = newMuxerVariantFMP4Part(
			s.videoTrack,
			s.audioTrack,
			s.genPartID(),
			s.storage.NewPart(),
		)
	}

	return nil
}

func (s *muxerVariantFMP4Segment) writeAudio(sample *augmentedAudioSample, adjustedPartDuration time.Duration) error {
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

		s.currentPart = newMuxerVariantFMP4Part(
			s.videoTrack,
			s.audioTrack,
			s.genPartID(),
			s.storage.NewPart(),
		)
	}

	return nil
}
