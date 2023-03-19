package gohlslib

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h264"
	"github.com/aler9/gortsplib/v2/pkg/format"

	"github.com/bluenviron/gohlslib/pkg/mpegts"
	"github.com/bluenviron/gohlslib/pkg/storage"
)

const (
	mpegtsSegmentMinAUCount = 100
)

type muxerSegmenterMPEGTS struct {
	segmentDuration time.Duration
	segmentMaxSize  uint64
	videoTrack      *format.H264
	audioTrack      *format.MPEG4Audio
	factory         storage.Factory
	onSegmentReady  func(muxerSegment)

	writer            *mpegts.Writer
	nextSegmentID     uint64
	currentSegment    *muxerSegmentMPEGTS
	videoDTSExtractor *h264.DTSExtractor
	startPCR          time.Time
	startDTS          time.Duration
}

func newMuxerSegmenterMPEGTS(
	segmentDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack *format.H264,
	audioTrack *format.MPEG4Audio,
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

	m.writer = mpegts.NewWriter(
		videoTrack,
		audioTrack)

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

func (m *muxerSegmenterMPEGTS) writeH26x(ntp time.Time, pts time.Duration, nalus [][]byte) error {
	idrPresent := false
	nonIDRPresent := false

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeIDR:
			idrPresent = true

		case h264.NALUTypeNonIDR:
			nonIDRPresent = true
		}
	}

	var dts time.Duration

	if m.currentSegment == nil {
		// skip groups silently until we find one with a IDR
		if !idrPresent {
			return nil
		}

		m.videoDTSExtractor = h264.NewDTSExtractor()

		var err error
		dts, err = m.videoDTSExtractor.Extract(nalus, pts)
		if err != nil {
			return fmt.Errorf("unable to extract DTS: %v", err)
		}

		m.startPCR = ntp
		m.startDTS = dts
		dts = 0
		pts -= m.startDTS

		// create first segment
		m.currentSegment, err = newMuxerSegmentMPEGTS(
			m.genSegmentID(),
			ntp,
			m.segmentMaxSize,
			m.videoTrack,
			m.audioTrack,
			m.writer,
			m.factory)
		if err != nil {
			return err
		}
	} else {
		if !idrPresent && !nonIDRPresent {
			return nil
		}

		var err error
		dts, err = m.videoDTSExtractor.Extract(nalus, pts)
		if err != nil {
			return fmt.Errorf("unable to extract DTS: %v", err)
		}

		dts -= m.startDTS
		pts -= m.startDTS

		// switch segment
		if idrPresent &&
			(dts-*m.currentSegment.startDTS) >= m.segmentDuration {
			m.currentSegment.finalize(dts)
			m.onSegmentReady(m.currentSegment)

			var err error
			m.currentSegment, err = newMuxerSegmentMPEGTS(
				m.genSegmentID(),
				ntp,
				m.segmentMaxSize,
				m.videoTrack,
				m.audioTrack,
				m.writer,
				m.factory,
			)
			if err != nil {
				return err
			}
		}
	}

	err := m.currentSegment.writeH264(
		ntp.Sub(m.startPCR),
		dts,
		pts,
		idrPresent,
		nalus)
	if err != nil {
		return err
	}

	return nil
}

func (m *muxerSegmenterMPEGTS) writeAudio(ntp time.Time, pts time.Duration, au []byte) error {
	if m.videoTrack == nil {
		if m.currentSegment == nil {
			m.startPCR = ntp
			m.startDTS = pts
			pts = 0

			// create first segment
			var err error
			m.currentSegment, err = newMuxerSegmentMPEGTS(
				m.genSegmentID(),
				ntp,
				m.segmentMaxSize,
				m.videoTrack,
				m.audioTrack,
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
					m.videoTrack,
					m.audioTrack,
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

	err := m.currentSegment.writeAAC(ntp.Sub(m.startPCR), pts, au)
	if err != nil {
		return err
	}

	return nil
}
