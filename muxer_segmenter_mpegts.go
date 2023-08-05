package gohlslib

import (
	"fmt"
	"io"
	"time"

	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"

	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/gohlslib/pkg/storage"
)

const (
	mpegtsSegmentMinAUCount = 100
)

type switchableWriter struct {
	w io.Writer
}

func (w *switchableWriter) Write(p []byte) (int, error) {
	return w.w.Write(p)
}

type muxerSegmenterMPEGTS struct {
	segmentDuration time.Duration
	segmentMaxSize  uint64
	videoTrack      *Track
	audioTrack      *Track
	factory         storage.Factory
	onSegmentReady  func(muxerSegment)

	writerVideoTrack  *mpegts.Track
	writerAudioTrack  *mpegts.Track
	switchableWriter  *switchableWriter
	writer            *mpegts.Writer
	nextSegmentID     uint64
	currentSegment    *muxerSegmentMPEGTS
	videoDTSExtractor *h264.DTSExtractor
	startDTS          time.Duration
}

func newMuxerSegmenterMPEGTS(
	segmentDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack *Track,
	audioTrack *Track,
	factory storage.Factory,
	onSegmentReady func(muxerSegment),
) *muxerSegmenterMPEGTS {
	m := &muxerSegmenterMPEGTS{
		segmentDuration: segmentDuration,
		segmentMaxSize:  segmentMaxSize,
		videoTrack:      videoTrack,
		audioTrack:      audioTrack,
		factory:         factory,
		onSegmentReady:  onSegmentReady,
	}

	var tracks []*mpegts.Track

	if videoTrack != nil {
		m.writerVideoTrack = &mpegts.Track{
			Codec: codecs.ToMPEGTS(videoTrack.Codec),
		}
		tracks = append(tracks, m.writerVideoTrack)
	}

	if audioTrack != nil {
		m.writerAudioTrack = &mpegts.Track{
			Codec: codecs.ToMPEGTS(audioTrack.Codec),
		}
		tracks = append(tracks, m.writerAudioTrack)
	}

	m.switchableWriter = &switchableWriter{}

	m.writer = mpegts.NewWriter(m.switchableWriter, tracks)

	return m
}

func (m *muxerSegmenterMPEGTS) close() {
	if m.currentSegment != nil {
		m.currentSegment.finalize(0)
		m.currentSegment.close()
	}
}

func (m *muxerSegmenterMPEGTS) genSegmentID() uint64 {
	id := m.nextSegmentID
	m.nextSegmentID++
	return id
}

func (m *muxerSegmenterMPEGTS) writeAV1(
	_ time.Time,
	_ time.Duration,
	_ [][]byte,
	_ bool,
	_ bool,
) error {
	return fmt.Errorf("unimplemented")
}

func (m *muxerSegmenterMPEGTS) writeH26x(
	ntp time.Time,
	pts time.Duration,
	au [][]byte,
	randomAccessPresent bool,
	forceSwitch bool,
) error {
	var dts time.Duration

	if m.currentSegment == nil {
		// skip groups silently until we find one with a IDR
		if !randomAccessPresent {
			return nil
		}

		m.videoDTSExtractor = h264.NewDTSExtractor()

		var err error
		dts, err = m.videoDTSExtractor.Extract(au, pts)
		if err != nil {
			return fmt.Errorf("unable to extract DTS: %v", err)
		}

		m.startDTS = dts
		dts = 0
		pts -= m.startDTS

		// create first segment
		m.currentSegment, err = newMuxerSegmentMPEGTS(
			m.genSegmentID(),
			ntp,
			m.segmentMaxSize,
			m.writerVideoTrack,
			m.writerAudioTrack,
			m.switchableWriter,
			m.writer,
			m.factory)
		if err != nil {
			return err
		}
	} else {
		var err error
		dts, err = m.videoDTSExtractor.Extract(au, pts)
		if err != nil {
			return fmt.Errorf("unable to extract DTS: %v", err)
		}

		dts -= m.startDTS
		pts -= m.startDTS

		// switch segment
		if randomAccessPresent &&
			((dts-*m.currentSegment.startDTS) >= m.segmentDuration ||
				forceSwitch) {
			m.currentSegment.finalize(dts)
			m.onSegmentReady(m.currentSegment)

			var err error
			m.currentSegment, err = newMuxerSegmentMPEGTS(
				m.genSegmentID(),
				ntp,
				m.segmentMaxSize,
				m.writerVideoTrack,
				m.writerAudioTrack,
				m.switchableWriter,
				m.writer,
				m.factory,
			)
			if err != nil {
				return err
			}
		}
	}

	err := m.currentSegment.writeH264(
		pts,
		dts,
		randomAccessPresent,
		au)
	if err != nil {
		return err
	}

	return nil
}

func (m *muxerSegmenterMPEGTS) writeOpus(_ time.Time, _ time.Duration, _ [][]byte) error {
	return fmt.Errorf("unimplemented")
}

func (m *muxerSegmenterMPEGTS) writeMPEG4Audio(ntp time.Time, pts time.Duration, aus [][]byte) error {
	return m.writeAudio(ntp, pts, aus)
}

func (m *muxerSegmenterMPEGTS) writeAudio(ntp time.Time, pts time.Duration, aus [][]byte) error {
	if m.videoTrack == nil {
		if m.currentSegment == nil {
			m.startDTS = pts
			pts = 0

			// create first segment
			var err error
			m.currentSegment, err = newMuxerSegmentMPEGTS(
				m.genSegmentID(),
				ntp,
				m.segmentMaxSize,
				m.writerVideoTrack,
				m.writerAudioTrack,
				m.switchableWriter,
				m.writer,
				m.factory,
			)
			if err != nil {
				return err
			}
		} else {
			pts -= m.startDTS

			// switch segment
			if m.currentSegment.audioAUCount >= mpegtsSegmentMinAUCount &&
				(pts-*m.currentSegment.startDTS) >= m.segmentDuration {
				m.currentSegment.finalize(pts)
				m.onSegmentReady(m.currentSegment)

				var err error
				m.currentSegment, err = newMuxerSegmentMPEGTS(
					m.genSegmentID(),
					ntp,
					m.segmentMaxSize,
					m.writerVideoTrack,
					m.writerAudioTrack,
					m.switchableWriter,
					m.writer,
					m.factory,
				)
				if err != nil {
					return err
				}
			}
		}
	} else {
		// wait for the video track
		if m.currentSegment == nil {
			return nil
		}

		pts -= m.startDTS
	}

	err := m.currentSegment.writeMPEG4Audio(pts, aus)
	if err != nil {
		return err
	}

	return nil
}
