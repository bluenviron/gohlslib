package gohlslib

import (
	"fmt"
	"time"

	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/pkg/codecs/opus"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"

	"github.com/bluenviron/gohlslib/pkg/blankvideo"
	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/gohlslib/pkg/storage"
)

const (
	fmp4StartDTS     = 10 * time.Second
	blankSegmentsFPS = time.Duration(15)
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

type augmentedSample struct {
	fmp4.PartSample
	dts time.Duration
	ntp time.Time
}

type muxerSegmenterFMP4 struct {
	lowLatency           bool
	segmentCount         int
	segmentMinDuration   time.Duration
	partMinDuration      time.Duration
	segmentMaxSize       uint64
	videoTrack           *Track
	audioTrack           *Track
	prefix               string
	factory              storage.Factory
	parentPublishSegment func(muxerSegment) error
	parentPublishPart    func(*muxerPart) error

	audioTimeScale                 uint32
	videoFirstRandomAccessReceived bool
	videoDTSExtractor              dtsExtractor
	startDTS                       time.Duration
	currentSegment                 *muxerSegmentFMP4
	nextPartID                     uint64
	nextSegmentID                  uint64
	nextVideoSample                *augmentedSample
	nextAudioSample                *augmentedSample
	firstSegmentPublished          bool
	sampleDurations                map[time.Duration]struct{}
	adjustedPartDuration           time.Duration
}

func (m *muxerSegmenterFMP4) initialize() error {
	m.sampleDurations = make(map[time.Duration]struct{})

	if m.audioTrack != nil {
		m.audioTimeScale = fmp4TimeScale(m.audioTrack.Codec)
	}

	err := m.generateBlankSegments()
	if err != nil {
		return err
	}

	return nil
}

func (m *muxerSegmenterFMP4) close() {
	if m.currentSegment != nil {
		m.currentSegment.finalize(0) //nolint:errcheck
		m.currentSegment.close()
	}
}

func (m *muxerSegmenterFMP4) takeSegmentID() uint64 {
	id := m.nextSegmentID
	m.nextSegmentID++
	return id
}

func (m *muxerSegmenterFMP4) takePartID() uint64 {
	id := m.nextPartID
	m.nextPartID++
	return id
}

func (m *muxerSegmenterFMP4) givePartID() {
	m.nextPartID--
}

func (m *muxerSegmenterFMP4) blankSegmentsCount() int {
	return m.segmentCount
}

func (m *muxerSegmenterFMP4) blankSegmentsDuration() time.Duration {
	return time.Duration(m.blankSegmentsCount()) * m.segmentMinDuration
}

// iPhone iOS fails if part durations are less than 85% of maximum part duration.
// find a part duration that is compatible with all received sample durations
func (m *muxerSegmenterFMP4) adjustPartDuration(sampleDuration time.Duration) {
	if !m.lowLatency || m.firstSegmentPublished {
		return
	}

	// avoid a crash by skipping invalid durations
	if sampleDuration == 0 {
		return
	}

	if _, ok := m.sampleDurations[sampleDuration]; !ok {
		m.sampleDurations[sampleDuration] = struct{}{}
		m.adjustedPartDuration = findCompatiblePartDuration(
			m.partMinDuration,
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

	return m.writeVideo(
		randomAccess,
		forceSwitch,
		&augmentedSample{
			PartSample: *ps,
			dts:        dts,
			ntp:        ntp,
		})
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

	return m.writeVideo(
		randomAccess,
		forceSwitch,
		&augmentedSample{
			PartSample: fmp4.PartSample{
				IsNonSyncSample: !randomAccess,
				Payload:         frame,
			},
			dts: dts,
			ntp: ntp,
		})
}

func (m *muxerSegmenterFMP4) writeH26x(
	ntp time.Time,
	pts time.Duration,
	au [][]byte,
	randomAccess bool,
	forceSwitch bool,
) error {
	// var sps h264.SPS
	// sps.Unmarshal(m.videoTrack.Codec.(*codecs.H264).SPS)

	/*
		fmt.Println("{")
		for _, nalu := range au {
			fmt.Println("{")
			for i, x := range nalu {
				fmt.Printf("0x%.2x, ", x)
				if (i+1)%8 == 0 {
					fmt.Println("")
				}
			}

			fmt.Println("},")
		}
		fmt.Println("},")
	*/

	if !m.videoFirstRandomAccessReceived {
		// skip sample silently until we find one with an IDR
		if !randomAccess {
			return nil
		}

		m.videoFirstRandomAccessReceived = true
		m.videoDTSExtractor = allocateDTSExtractor(m.videoTrack)
	}

	dts, err := m.videoDTSExtractor.Extract(au, pts)
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

	return m.writeVideo(
		randomAccess,
		forceSwitch,
		&augmentedSample{
			PartSample: *ps,
			dts:        dts,
			ntp:        ntp,
		})
}

func (m *muxerSegmenterFMP4) writeOpus(ntp time.Time, pts time.Duration, packets [][]byte) error {
	for _, packet := range packets {
		err := m.writeAudio(&augmentedSample{
			PartSample: fmp4.PartSample{
				Payload: packet,
			},
			dts: pts,
			ntp: ntp,
		})
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

		err := m.writeAudio(&augmentedSample{
			PartSample: fmp4.PartSample{
				Payload: au,
			},
			dts: auPTS,
			ntp: auNTP,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *muxerSegmenterFMP4) writeVideo(
	randomAccess bool,
	forceSwitch bool,
	sample *augmentedSample,
) error {
	// put samples into a queue in order to
	// - compute sample duration
	// - check if next sample is IDR
	sample, m.nextVideoSample = m.nextVideoSample, sample
	if sample == nil {
		return nil
	}
	duration := m.nextVideoSample.dts - sample.dts
	sample.Duration = uint32(durationGoToMp4(duration, 90000))

	// create first segment
	if m.currentSegment == nil {
		m.startDTS = sample.dts

		m.currentSegment = &muxerSegmentFMP4{
			lowLatency:     m.lowLatency,
			id:             m.takeSegmentID(),
			startNTP:       &sample.ntp,
			startDTS:       m.blankSegmentsDuration() + fmp4StartDTS,
			segmentMaxSize: m.segmentMaxSize,
			videoTrack:     m.videoTrack,
			audioTrack:     m.audioTrack,
			audioTimeScale: m.audioTimeScale,
			prefix:         m.prefix,
			forceSwitched:  false,
			factory:        m.factory,
			takePartID:     m.takePartID,
			givePartID:     m.givePartID,
			publishPart:    m.publishPart,
		}
		err := m.currentSegment.initialize()
		if err != nil {
			return err
		}
	}

	// add a starting DTS to avoid a negative BaseTime
	sample.dts += m.blankSegmentsDuration() + fmp4StartDTS - m.startDTS

	// BaseTime is still negative, this is not supported by fMP4. Reject the sample silently.
	if sample.dts < 0 {
		return nil
	}

	m.adjustPartDuration(duration)

	nextDTS := sample.dts + duration

	err := m.currentSegment.writeVideo(sample, nextDTS, m.adjustedPartDuration)
	if err != nil {
		return err
	}

	// switch segment
	if randomAccess &&
		((nextDTS-m.currentSegment.startDTS) >= m.segmentMinDuration ||
			forceSwitch) {
		err := m.currentSegment.finalize(nextDTS)
		if err != nil {
			return err
		}

		err = m.publishSegment(m.currentSegment)
		if err != nil {
			return err
		}

		m.currentSegment = &muxerSegmentFMP4{
			lowLatency:     m.lowLatency,
			id:             m.takeSegmentID(),
			startNTP:       &m.nextVideoSample.ntp,
			startDTS:       nextDTS,
			segmentMaxSize: m.segmentMaxSize,
			videoTrack:     m.videoTrack,
			audioTrack:     m.audioTrack,
			audioTimeScale: m.audioTimeScale,
			prefix:         m.prefix,
			forceSwitched:  forceSwitch,
			factory:        m.factory,
			takePartID:     m.takePartID,
			givePartID:     m.givePartID,
			publishPart:    m.publishPart,
		}
		err = m.currentSegment.initialize()
		if err != nil {
			return err
		}

		if forceSwitch {
			m.firstSegmentPublished = false

			// reset adjusted part duration
			m.sampleDurations = make(map[time.Duration]struct{})
		}
	}

	return nil
}

func (m *muxerSegmenterFMP4) writeAudio(sample *augmentedSample) error {
	// put samples into a queue in order to compute the sample duration
	sample, m.nextAudioSample = m.nextAudioSample, sample
	if sample == nil {
		return nil
	}
	duration := m.nextAudioSample.dts - sample.dts
	sample.Duration = uint32(durationGoToMp4(duration, m.audioTimeScale))

	if m.videoTrack == nil {
		// create first segment
		if m.currentSegment == nil {
			m.startDTS = sample.dts

			m.currentSegment = &muxerSegmentFMP4{
				lowLatency:     m.lowLatency,
				id:             m.takeSegmentID(),
				startNTP:       &sample.ntp,
				startDTS:       m.blankSegmentsDuration() + fmp4StartDTS,
				segmentMaxSize: m.segmentMaxSize,
				videoTrack:     m.videoTrack,
				audioTrack:     m.audioTrack,
				audioTimeScale: m.audioTimeScale,
				prefix:         m.prefix,
				forceSwitched:  false,
				factory:        m.factory,
				takePartID:     m.takePartID,
				givePartID:     m.givePartID,
				publishPart:    m.publishPart,
			}
			err := m.currentSegment.initialize()
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

	// add a starting DTS to avoid a negative BaseTime
	sample.dts += m.blankSegmentsDuration() + fmp4StartDTS - m.startDTS

	// BaseTime is still negative, this is not supported by fMP4. Reject the sample silently.
	if sample.dts < 0 {
		return nil
	}

	nextDTS := sample.dts + duration

	err := m.currentSegment.writeAudio(sample, nextDTS, m.partMinDuration)
	if err != nil {
		return err
	}

	// switch segment
	if m.videoTrack == nil &&
		(nextDTS-m.currentSegment.startDTS) >= m.segmentMinDuration {
		err := m.currentSegment.finalize(nextDTS)
		if err != nil {
			return err
		}

		err = m.publishSegment(m.currentSegment)
		if err != nil {
			return err
		}

		m.currentSegment = &muxerSegmentFMP4{
			lowLatency:     m.lowLatency,
			id:             m.takeSegmentID(),
			startNTP:       &m.nextAudioSample.ntp,
			startDTS:       nextDTS,
			segmentMaxSize: m.segmentMaxSize,
			videoTrack:     m.videoTrack,
			audioTrack:     m.audioTrack,
			audioTimeScale: m.audioTimeScale,
			prefix:         m.prefix,
			forceSwitched:  false,
			factory:        m.factory,
			takePartID:     m.takePartID,
			givePartID:     m.givePartID,
			publishPart:    m.publishPart,
		}
		err = m.currentSegment.initialize()
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *muxerSegmenterFMP4) generateBlankSegments() error {
	firstDTS := fmp4StartDTS
	finalDTS := firstDTS + m.blankSegmentsDuration()
	dts := firstDTS

	fmt.Println("FIRST DTS", dts)

	for i := 0; i < m.blankSegmentsCount(); i++ {
		fmt.Println("STARTD", dts)
		seg := &muxerSegmentFMP4{
			lowLatency: m.lowLatency,
			id:         m.takeSegmentID(),
			// startNTP:       nil,
			startDTS:       dts,
			segmentMaxSize: m.segmentMaxSize,
			videoTrack:     m.videoTrack,
			audioTrack:     m.audioTrack,
			audioTimeScale: m.audioTimeScale,
			prefix:         m.prefix,
			forceSwitched:  false,
			factory:        m.factory,
			takePartID:     m.takePartID,
			givePartID:     m.givePartID,
			publishPart:    m.publishPart,
		}
		err := seg.initialize()
		if err != nil {
			return err
		}

		v := blankvideo.H264{}

		fmt.Println("FIRSTSEGDTS", dts)

		for (dts - firstDTS) < (m.segmentMinDuration * time.Duration(i+1)) {
			au := v.AccessUnit()

			ps, err := fmp4.NewPartSampleH26x(
				0,
				h264.IDRPresent(au),
				au)
			if err != nil {
				return err
			}
			ps.Duration = uint32(durationGoToMp4(time.Second/blankSegmentsFPS, 90000))

			err = seg.writeVideo(
				&augmentedSample{
					PartSample: *ps,
					dts:        dts,
					ntp:        time.Time{},
				},
				dts+(time.Second/blankSegmentsFPS),
				m.partMinDuration,
			)
			if err != nil {
				return err
			}

			dts += (time.Second / blankSegmentsFPS)
		}

		if dts < finalDTS {
			err = seg.finalize(dts)
			if err != nil {
				return err
			}
		} else {
			fmt.Println("H")
			err = seg.finalize(finalDTS)
			if err != nil {
				return err
			}
		}

		err = m.publishSegment(seg)
		if err != nil {
			return err
		}
	}

	fmt.Println("LAST DTS", dts, firstDTS, finalDTS)

	return nil
}

func (m *muxerSegmenterFMP4) publishPart(part *muxerPart) error {
	return m.parentPublishPart(part)
}

func (m *muxerSegmenterFMP4) publishSegment(segment *muxerSegmentFMP4) error {
	m.firstSegmentPublished = true
	return m.parentPublishSegment(segment)
}
