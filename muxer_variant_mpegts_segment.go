package gohlslib

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"

	"github.com/bluenviron/gohlslib/pkg/mpegts"
	"github.com/bluenviron/gohlslib/pkg/storage"
)

type muxerVariantMPEGTSSegment struct {
	segmentMaxSize uint64
	videoTrack     *format.H264
	audioTrack     *format.MPEG4Audio
	writer         *mpegts.Writer

	storage      storage.Segment
	storagePart  storage.Part
	size         uint64
	startTime    time.Time
	name         string
	startDTS     *time.Duration
	endDTS       time.Duration
	audioAUCount int
}

func newMuxerVariantMPEGTSSegment(
	id uint64,
	startTime time.Time,
	segmentMaxSize uint64,
	videoTrack *format.H264,
	audioTrack *format.MPEG4Audio,
	writer *mpegts.Writer,
	factory storage.Factory,
) (*muxerVariantMPEGTSSegment, error) {
	s := &muxerVariantMPEGTSSegment{
		segmentMaxSize: segmentMaxSize,
		videoTrack:     videoTrack,
		audioTrack:     audioTrack,
		writer:         writer,
		startTime:      startTime,
		name:           "seg" + strconv.FormatUint(id, 10),
	}

	var err error
	s.storage, err = factory.NewSegment(s.name + ".ts")
	if err != nil {
		return nil, err
	}

	s.storagePart = s.storage.NewPart()

	writer.SetByteWriter(s.storagePart.Writer())

	return s, nil
}

func (t *muxerVariantMPEGTSSegment) close() {
	t.storage.Remove()
}

func (t *muxerVariantMPEGTSSegment) duration() time.Duration {
	return t.endDTS - *t.startDTS
}

func (t *muxerVariantMPEGTSSegment) reader() (io.ReadCloser, error) {
	return t.storage.Reader()
}

func (t *muxerVariantMPEGTSSegment) finalize(endDTS time.Duration) {
	t.endDTS = endDTS
	t.storage.Finalize()
}

func (t *muxerVariantMPEGTSSegment) writeH264(
	pcr time.Duration,
	dts time.Duration,
	pts time.Duration,
	idrPresent bool,
	nalus [][]byte,
) error {
	size := uint64(0)
	for _, nalu := range nalus {
		size += uint64(len(nalu))
	}
	if (t.size + size) > t.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	t.size += size

	err := t.writer.WriteH264(pcr, dts, pts, idrPresent, nalus)
	if err != nil {
		return err
	}

	if t.startDTS == nil {
		t.startDTS = &dts
	}
	t.endDTS = dts

	return nil
}

func (t *muxerVariantMPEGTSSegment) writeAAC(
	pcr time.Duration,
	pts time.Duration,
	au []byte,
) error {
	size := uint64(len(au))
	if (t.size + size) > t.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	t.size += size

	err := t.writer.WriteAAC(pcr, pts, au)
	if err != nil {
		return err
	}

	if t.videoTrack == nil {
		t.audioAUCount++

		if t.startDTS == nil {
			t.startDTS = &pts
		}
		t.endDTS = pts
	}

	return nil
}
