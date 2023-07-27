package gohlslib

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/bluenviron/gohlslib/pkg/storage"

	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
)

func durationGoToMPEGTS(v time.Duration) int64 {
	return int64(v.Seconds() * 90000)
}

type muxerSegmentMPEGTS struct {
	segmentMaxSize   uint64
	writerVideoTrack *mpegts.Track
	writerAudioTrack *mpegts.Track
	writer           *mpegts.Writer

	storage      storage.File
	storagePart  storage.Part
	bw           *bufio.Writer
	size         uint64
	startNTP     time.Time
	name         string
	startDTS     *time.Duration
	endDTS       time.Duration
	audioAUCount int
}

func newMuxerSegmentMPEGTS(
	id uint64,
	startNTP time.Time,
	segmentMaxSize uint64,
	writerVideoTrack *mpegts.Track,
	writerAudioTrack *mpegts.Track,
	switchableWriter *switchableWriter,
	writer *mpegts.Writer,
	factory storage.Factory,
) (*muxerSegmentMPEGTS, error) {
	t := &muxerSegmentMPEGTS{
		segmentMaxSize:   segmentMaxSize,
		writerVideoTrack: writerVideoTrack,
		writerAudioTrack: writerAudioTrack,
		writer:           writer,
		startNTP:         startNTP,
		name:             "seg" + strconv.FormatUint(id, 10),
	}

	var err error
	t.storage, err = factory.NewFile(t.name + ".ts")
	if err != nil {
		return nil, err
	}

	t.storagePart = t.storage.NewPart()
	t.bw = bufio.NewWriter(t.storagePart.Writer())

	switchableWriter.w = t.bw

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

func (t *muxerSegmentMPEGTS) finalize(nextDTS time.Duration) {
	t.endDTS = nextDTS
	t.bw.Flush()
	t.bw = nil
	t.storage.Finalize()
}

func (t *muxerSegmentMPEGTS) writeH264(
	dts time.Duration,
	pts time.Duration,
	idrPresent bool,
	au [][]byte,
) error {
	size := uint64(0)
	for _, nalu := range au {
		size += uint64(len(nalu))
	}
	if (t.size + size) > t.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	t.size += size

	// prepend an AUD. This is required by video.js and iOS
	au = append([][]byte{{byte(h264.NALUTypeAccessUnitDelimiter), 240}}, au...)

	err := t.writer.WriteH26x(t.writerVideoTrack, durationGoToMPEGTS(dts), durationGoToMPEGTS(pts), idrPresent, au)
	if err != nil {
		return err
	}

	if t.startDTS == nil {
		t.startDTS = &dts
	}
	t.endDTS = dts

	return nil
}

func (t *muxerSegmentMPEGTS) writeMPEG4Audio(
	pts time.Duration,
	aus [][]byte,
) error {
	size := uint64(0)
	for _, au := range aus {
		size += uint64(len(au))
	}

	if (t.size + size) > t.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	t.size += size

	err := t.writer.WriteMPEG4Audio(t.writerAudioTrack, durationGoToMPEGTS(pts), aus)
	if err != nil {
		return err
	}

	if t.writerVideoTrack == nil {
		t.audioAUCount++

		if t.startDTS == nil {
			t.startDTS = &pts
		}
		t.endDTS = pts
	}

	return nil
}
