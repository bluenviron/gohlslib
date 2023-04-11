package gohlslib

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/bluenviron/gohlslib/pkg/storage"

	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
)

type muxerSegmentMPEGTS struct {
	segmentMaxSize uint64
	hasVideoTrack  bool
	writer         *mpegts.Writer

	storage      storage.File
	storagePart  storage.Part
	bw           *bufio.Writer
	size         uint64
	startTime    time.Time
	name         string
	startDTS     *time.Duration
	endDTS       time.Duration
	audioAUCount int
}

func newMuxerSegmentMPEGTS(
	id uint64,
	startTime time.Time,
	segmentMaxSize uint64,
	hasVideoTrack bool,
	writer *mpegts.Writer,
	factory storage.Factory,
) (*muxerSegmentMPEGTS, error) {
	t := &muxerSegmentMPEGTS{
		segmentMaxSize: segmentMaxSize,
		hasVideoTrack:  hasVideoTrack,
		writer:         writer,
		startTime:      startTime,
		name:           "seg" + strconv.FormatUint(id, 10),
	}

	var err error
	t.storage, err = factory.NewFile(t.name + ".ts")
	if err != nil {
		return nil, err
	}

	t.storagePart = t.storage.NewPart()
	t.bw = bufio.NewWriter(t.storagePart.Writer())

	writer.SetByteWriter(t.bw)

	return t, nil
}

func (t *muxerSegmentMPEGTS) close() {
	t.storage.Remove()
}

func (t *muxerSegmentMPEGTS) getName() string {
	return t.name
}

func (t *muxerSegmentMPEGTS) getDuration() time.Duration {
	return t.endDTS - *t.startDTS
}

func (t *muxerSegmentMPEGTS) getSize() uint64 {
	return t.storage.Size()
}

func (t *muxerSegmentMPEGTS) reader() (io.ReadCloser, error) {
	return t.storage.Reader()
}

func (t *muxerSegmentMPEGTS) finalize(endDTS time.Duration) {
	t.endDTS = endDTS
	t.bw.Flush()
	t.bw = nil
	t.storage.Finalize()
}

func (t *muxerSegmentMPEGTS) writeH264(
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

func (t *muxerSegmentMPEGTS) writeAAC(
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

	if !t.hasVideoTrack {
		t.audioAUCount++

		if t.startDTS == nil {
			t.startDTS = &pts
		}
		t.endDTS = pts
	}

	return nil
}
