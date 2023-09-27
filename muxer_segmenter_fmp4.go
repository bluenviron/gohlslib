package gohlslib

import (
	"fmt"
	"time"

	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/pkg/codecs/opus"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"

	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/gohlslib/pkg/storage"
)

func fmp4TimeScale(c codecs.Codec) uint32 {
	switch codec := c.(type) {
	case *codecs.MPEG4Audio:
		return uint32(codec.SampleRate)

	case *codecs.Opus:
		return 48000
	}

	return 0
}

func partDurationIsCompatible(partDuration time.Duration, sampleDuration time.Duration) bool {
	if sampleDuration > partDuration {
		return false
	}

	f := (partDuration / sampleDuration)
	if (partDuration % sampleDuration) != 0 {
		f++
	}
	f *= sampleDuration

	return partDuration > ((f * 85) / 100)
}

func partDurationIsCompatibleWithAll(partDuration time.Duration, sampleDurations map[time.Duration]struct{}) bool {
	for sd := range sampleDurations {
		if !partDurationIsCompatible(partDuration, sd) {
			return false
		}
	}
	return true
}

func findCompatiblePartDuration(
	minPartDuration time.Duration,
	sampleDurations map[time.Duration]struct{},
) time.Duration {
	i := minPartDuration
	for ; i < 5*time.Second; i += 5 * time.Millisecond {
		if partDurationIsCompatibleWithAll(i, sampleDurations) {
			break
		}
	}
	return i
}

type dtsExtractor interface {
	Extract([][]byte, time.Duration) (time.Duration, error)
}

func allocateDTSExtractor(track *Track) dtsExtractor {
	switch track.Codec.(type) {
	case *codecs.H265:
		return h265.NewDTSExtractor()

	case *codecs.H264:
		return h264.NewDTSExtractor()
	}
	return nil
}

type augmentedVideoSample struct {
	fmp4.PartSample
	dts time.Duration
	ntp time.Time
}

type augmentedAudioSample struct {
	fmp4.PartSample
	dts time.Duration
	ntp time.Time
}

type muxerSegmenterFMP4 struct {
	lowLatency      bool
	segmentDuration time.Duration
	partDuration    time.Duration
	segmentMaxSize  uint64
	videoTrack      *Track
	audioTrack      *Track
	prefix          string
	factory         storage.Factory
	publishSegment  func(muxerSegment) error
	publishPart     func(*muxerPart)

	audioTimeScale                 uint32
	videoFirstRandomAccessReceived bool
	videoDTSExtractor              dtsExtractor
	currentSegment                 *muxerSegmentFMP4
	nextSegmentID                  uint64
	nextPartID                     uint64
	nextVideoSample                *augmentedVideoSample
	nextAudioSample                *augmentedAudioSample
	firstSegmentFinalized          bool
	sampleDurations                map[time.Duration]struct{}
	adjustedPartDuration           time.Duration
}

func newMuxerSegmenterFMP4(
	lowLatency bool,
	segmentDuration time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack *Track,
	audioTrack *Track,
	prefix string,
	factory storage.Factory,
	publishSegment func(muxerSegment) error,
	publishPart func(*muxerPart),
) *muxerSegmenterFMP4 {
	m := &muxerSegmenterFMP4{
		lowLatency:      lowLatency,
		segmentDuration: segmentDuration,
		partDuration:    partDuration,
		segmentMaxSize:  segmentMaxSize,
		videoTrack:      videoTrack,
		audioTrack:      audioTrack,
		prefix:          prefix,
		factory:         factory,
		publishSegment:  publishSegment,
		publishPart:     publishPart,
		sampleDurations: make(map[time.Duration]struct{}),
	}

	if audioTrack != nil {
		m.audioTimeScale = fmp4TimeScale(audioTrack.Codec)
	}

	// add initial gaps, required by iOS LL-HLS
	if m.lowLatency {
		m.nextSegmentID = 7
	}

	return m
}

func (m *muxerSegmenterFMP4) close() {
	if m.currentSegment != nil {
		m.currentSegment.finalize(0) //nolint:errcheck
		m.currentSegment.close()
	}
}

func (m *muxerSegmenterFMP4) genSegmentID() uint64 {
	id := m.nextSegmentID
	m.nextSegmentID++
	return id
}

func (m *muxerSegmenterFMP4) genPartID() uint64 {
	id := m.nextPartID
	m.nextPartID++
	return id
}

// iPhone iOS fails if part durations are less than 85% of maximum part duration.
// find a part duration that is compatible with all received sample durations
func (m *muxerSegmenterFMP4) adjustPartDuration(sampleDuration time.Duration) {
	if !m.lowLatency || m.firstSegmentFinalized {
		return
	}

	// avoid a crash by skipping invalid durations
	if sampleDuration == 0 {
		return
	}

	if _, ok := m.sampleDurations[sampleDuration]; !ok {
		m.sampleDurations[sampleDuration] = struct{}{}
		m.adjustedPartDuration = findCompatiblePartDuration(
			m.partDuration,
			m.sampleDurations,
		)
	}
}

func (m *muxerSegmenterFMP4) writeAV1(
	ntp time.Time,
	dts time.Duration,
	tu [][]byte,
	randomAccess bool,
	forceSwitch bool,
) error {
	if !m.videoFirstRandomAccessReceived {
		// skip sample silently until we find one with an IDR
		if !randomAccess {
			return nil
		}

		m.videoFirstRandomAccessReceived = true
	}

	ps, err := fmp4.NewPartSampleAV1(
		randomAccess,
		tu)
	if err != nil {
		return err
	}

	sample := &augmentedVideoSample{
		PartSample: *ps,
		dts:        dts,
		ntp:        ntp,
	}

	return m.writeVideo(
		randomAccess,
		forceSwitch,
		sample)
}

func (m *muxerSegmenterFMP4) writeVP9(
	ntp time.Time,
	dts time.Duration,
	frame []byte,
	randomAccess bool,
	forceSwitch bool,
) error {
	if !m.videoFirstRandomAccessReceived {
		// skip sample silently until we find one with an IDR
		if !randomAccess {
			return nil
		}

		m.videoFirstRandomAccessReceived = true
	}

	sample := &augmentedVideoSample{
		PartSample: fmp4.PartSample{
			IsNonSyncSample: !randomAccess,
			Payload:         frame,
		},
		dts: dts,
		ntp: ntp,
	}

	return m.writeVideo(
		randomAccess,
		forceSwitch,
		sample)
}

func (m *muxerSegmenterFMP4) writeH26x(
	ntp time.Time,
	pts time.Duration,
	au [][]byte,
	randomAccess bool,
	forceSwitch bool,
) error {
	var dts time.Duration

	if !m.videoFirstRandomAccessReceived {
		// skip sample silently until we find one with an IDR
		if !randomAccess {
			return nil
		}

		m.videoFirstRandomAccessReceived = true
		m.videoDTSExtractor = allocateDTSExtractor(m.videoTrack)
	}

	var err error
	dts, err = m.videoDTSExtractor.Extract(au, pts)
	if err != nil {
		return fmt.Errorf("unable to extract DTS: %v", err)
	}

	ps, err := fmp4.NewPartSampleH26x(
		int32(durationGoToMp4(pts-dts, 90000)),
		randomAccess,
		au)
	if err != nil {
		return err
	}

	sample := &augmentedVideoSample{
		PartSample: *ps,
		dts:        dts,
		ntp:        ntp,
	}

	return m.writeVideo(
		randomAccess,
		forceSwitch,
		sample)
}

func (m *muxerSegmenterFMP4) writeVideo(
	randomAccess bool,
	forceSwitch bool,
	sample *augmentedVideoSample,
) error {
	// put samples into a queue in order to
	// - compute sample duration
	// - check if next sample is IDR
	sample, m.nextVideoSample = m.nextVideoSample, sample
	if sample == nil {
		return nil
	}
	sample.Duration = uint32(durationGoToMp4(m.nextVideoSample.dts-sample.dts, 90000))

	if m.currentSegment == nil {
		// create first segment
		var err error
		m.currentSegment, err = newMuxerSegmentFMP4(
			m.lowLatency,
			m.genSegmentID(),
			sample.ntp,
			sample.dts,
			m.segmentMaxSize,
			m.videoTrack,
			m.audioTrack,
			m.audioTimeScale,
			m.prefix,
			false,
			m.factory,
			m.genPartID,
			m.publishPart,
		)
		if err != nil {
			return err
		}
	}

	m.adjustPartDuration(m.nextVideoSample.dts - sample.dts)

	err := m.currentSegment.writeVideo(sample, m.nextVideoSample.dts, m.adjustedPartDuration)
	if err != nil {
		return err
	}

	// switch segment
	if randomAccess &&
		((m.nextVideoSample.dts-m.currentSegment.startDTS) >= m.segmentDuration ||
			forceSwitch) {
		err := m.currentSegment.finalize(m.nextVideoSample.dts)
		if err != nil {
			return err
		}

		err = m.publishSegment(m.currentSegment)
		if err != nil {
			return err
		}

		m.firstSegmentFinalized = true

		m.currentSegment, err = newMuxerSegmentFMP4(
			m.lowLatency,
			m.genSegmentID(),
			m.nextVideoSample.ntp,
			m.nextVideoSample.dts,
			m.segmentMaxSize,
			m.videoTrack,
			m.audioTrack,
			m.audioTimeScale,
			m.prefix,
			forceSwitch,
			m.factory,
			m.genPartID,
			m.publishPart,
		)
		if err != nil {
			return err
		}

		if forceSwitch {
			m.firstSegmentFinalized = false

			// reset adjusted part duration
			m.sampleDurations = make(map[time.Duration]struct{})
		}
	}

	return nil
}

func (m *muxerSegmenterFMP4) writeOpus(ntp time.Time, pts time.Duration, packets [][]byte) error {
	for _, packet := range packets {
		err := m.writeAudio(ntp, pts, packet)
		if err != nil {
			return err
		}

		duration := opus.PacketDuration(packet)
		ntp = ntp.Add(duration)
		pts += duration
	}

	return nil
}

func (m *muxerSegmenterFMP4) writeMPEG4Audio(ntp time.Time, pts time.Duration, aus [][]byte) error {
	sampleRate := time.Duration(m.audioTrack.Codec.(*codecs.MPEG4Audio).Config.SampleRate)

	for i, au := range aus {
		auNTP := ntp.Add(time.Duration(i) * mpeg4audio.SamplesPerAccessUnit *
			time.Second / sampleRate)
		auPTS := pts + time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*
			time.Second/sampleRate

		err := m.writeAudio(auNTP, auPTS, au)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *muxerSegmenterFMP4) writeAudio(ntp time.Time, dts time.Duration, au []byte) error {
	if m.videoTrack != nil {
		// wait for the video track
		if !m.videoFirstRandomAccessReceived {
			return nil
		}
	}

	sample := &augmentedAudioSample{
		PartSample: fmp4.PartSample{
			Payload: au,
		},
		dts: dts,
		ntp: ntp,
	}

	// put samples into a queue in order to compute the sample duration
	sample, m.nextAudioSample = m.nextAudioSample, sample
	if sample == nil {
		return nil
	}
	sample.Duration = uint32(durationGoToMp4(m.nextAudioSample.dts-sample.dts, m.audioTimeScale))

	if m.videoTrack == nil {
		if m.currentSegment == nil {
			// create first segment
			var err error
			m.currentSegment, err = newMuxerSegmentFMP4(
				m.lowLatency,
				m.genSegmentID(),
				sample.ntp,
				sample.dts,
				m.segmentMaxSize,
				m.videoTrack,
				m.audioTrack,
				m.audioTimeScale,
				m.prefix,
				false,
				m.factory,
				m.genPartID,
				m.publishPart,
			)
			if err != nil {
				return err
			}
		}
	} else {
		// wait for the video track
		if m.currentSegment == nil {
			return nil
		}
	}

	err := m.currentSegment.writeAudio(sample, m.nextAudioSample.dts, m.partDuration)
	if err != nil {
		return err
	}

	// switch segment
	if m.videoTrack == nil &&
		(m.nextAudioSample.dts-m.currentSegment.startDTS) >= m.segmentDuration {
		err := m.currentSegment.finalize(m.nextAudioSample.dts)
		if err != nil {
			return err
		}

		err = m.publishSegment(m.currentSegment)
		if err != nil {
			return err
		}

		m.firstSegmentFinalized = true

		m.currentSegment, err = newMuxerSegmentFMP4(
			m.lowLatency,
			m.genSegmentID(),
			m.nextAudioSample.ntp,
			m.nextAudioSample.dts,
			m.segmentMaxSize,
			m.videoTrack,
			m.audioTrack,
			m.audioTimeScale,
			m.prefix,
			false,
			m.factory,
			m.genPartID,
			m.publishPart,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
