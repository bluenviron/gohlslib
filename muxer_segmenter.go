package gohlslib

import (
	"bytes"
	"fmt"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/mediacommon/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/pkg/codecs/opus"
	"github.com/bluenviron/mediacommon/pkg/codecs/vp9"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
)

func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

func multiplyAndDivide2(v, m, d time.Duration) time.Duration {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

func durationToTimestamp(d time.Duration, clockRate int) int64 {
	return multiplyAndDivide(int64(d), int64(clockRate), int64(time.Second))
}

func timestampToDuration(d int64, clockRate int) time.Duration {
	return multiplyAndDivide2(time.Duration(d), time.Second, time.Duration(clockRate))
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

type fmp4AugmentedSample struct {
	fmp4.PartSample
	dts int64
	ntp time.Time
}

type muxerSegmenter struct {
	muxer *Muxer // TODO: remove

	pendingParamsChange            bool
	fmp4SampleDurations            map[time.Duration]struct{} // low-latency only
	fmp4AdjustedPartDuration       time.Duration              // low-latency only
	fmp4FreezeAdjustedPartDuration bool                       // low-latency only
}

func (s *muxerSegmenter) initialize() {
	if s.muxer.Variant != MuxerVariantMPEGTS {
		s.fmp4SampleDurations = make(map[time.Duration]struct{})
	}
}

func (s *muxerSegmenter) writeAV1(
	track *muxerTrack,
	ntp time.Time,
	pts int64,
	tu [][]byte,
) error {
	codec := track.Codec.(*codecs.AV1)
	randomAccess := false

	for _, obu := range tu {
		var h av1.OBUHeader
		err := h.Unmarshal(obu)
		if err != nil {
			return err
		}

		if h.Type == av1.OBUTypeSequenceHeader {
			randomAccess = true

			if !bytes.Equal(codec.SequenceHeader, obu) {
				s.pendingParamsChange = true
				codec.SequenceHeader = obu
			}
		}
	}

	paramsChanged := false
	if randomAccess && s.pendingParamsChange {
		s.pendingParamsChange = false
		paramsChanged = true
	}

	ps, err := fmp4.NewPartSampleAV1(
		randomAccess,
		tu)
	if err != nil {
		return err
	}

	return s.fmp4WriteSample(
		track,
		randomAccess,
		paramsChanged,
		&fmp4AugmentedSample{
			PartSample: *ps,
			dts:        pts,
			ntp:        ntp,
		})
}

func (s *muxerSegmenter) writeVP9(
	track *muxerTrack,
	ntp time.Time,
	pts int64,
	frame []byte,
) error {
	var h vp9.Header
	err := h.Unmarshal(frame)
	if err != nil {
		return err
	}

	codec := track.Codec.(*codecs.VP9)
	randomAccess := false

	if !h.NonKeyFrame {
		randomAccess = true

		if v := h.Width(); v != codec.Width {
			s.pendingParamsChange = true
			codec.Width = v
		}
		if v := h.Height(); v != codec.Height {
			s.pendingParamsChange = true
			codec.Height = v
		}
		if h.Profile != codec.Profile {
			s.pendingParamsChange = true
			codec.Profile = h.Profile
		}
		if h.ColorConfig.BitDepth != codec.BitDepth {
			s.pendingParamsChange = true
			codec.BitDepth = h.ColorConfig.BitDepth
		}
		if v := h.ChromaSubsampling(); v != codec.ChromaSubsampling {
			s.pendingParamsChange = true
			codec.ChromaSubsampling = v
		}
		if h.ColorConfig.ColorRange != codec.ColorRange {
			s.pendingParamsChange = true
			codec.ColorRange = h.ColorConfig.ColorRange
		}
	}

	paramsChanged := false
	if randomAccess && s.pendingParamsChange {
		s.pendingParamsChange = false
		paramsChanged = true
	}

	// skip samples silently until we find a random access one
	if !track.firstRandomAccessReceived {
		if !randomAccess {
			return nil
		}
		track.firstRandomAccessReceived = true
	}

	return s.fmp4WriteSample(
		track,
		randomAccess,
		paramsChanged,
		&fmp4AugmentedSample{
			PartSample: fmp4.PartSample{
				IsNonSyncSample: !randomAccess,
				Payload:         frame,
			},
			dts: pts,
			ntp: ntp,
		})
}

func (s *muxerSegmenter) writeH265(
	track *muxerTrack,
	ntp time.Time,
	pts int64,
	au [][]byte,
) error {
	randomAccess := false
	codec := track.Codec.(*codecs.H265)

	for _, nalu := range au {
		typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case h265.NALUType_IDR_W_RADL, h265.NALUType_IDR_N_LP, h265.NALUType_CRA_NUT:
			randomAccess = true

		case h265.NALUType_VPS_NUT:
			if !bytes.Equal(codec.VPS, nalu) {
				s.pendingParamsChange = true
				codec.VPS = nalu
			}

		case h265.NALUType_SPS_NUT:
			if !bytes.Equal(codec.SPS, nalu) {
				s.pendingParamsChange = true
				codec.SPS = nalu
			}

		case h265.NALUType_PPS_NUT:
			if !bytes.Equal(codec.PPS, nalu) {
				s.pendingParamsChange = true
				codec.PPS = nalu
			}
		}
	}

	paramsChanged := false
	if randomAccess && s.pendingParamsChange {
		s.pendingParamsChange = false
		paramsChanged = true
	}

	// skip samples silently until we find a random access one
	if !track.firstRandomAccessReceived {
		if !randomAccess {
			return nil
		}
		track.firstRandomAccessReceived = true

		track.h265DTSExtractor = h265.NewDTSExtractor2()
	}

	dts, err := track.h265DTSExtractor.Extract(au, pts)
	if err != nil {
		return fmt.Errorf("unable to extract DTS: %w", err)
	}

	ps, err := fmp4.NewPartSampleH26x(
		int32(pts-dts),
		randomAccess,
		au)
	if err != nil {
		return err
	}

	return s.fmp4WriteSample(
		track,
		randomAccess,
		paramsChanged,
		&fmp4AugmentedSample{
			PartSample: *ps,
			dts:        dts,
			ntp:        ntp,
		})
}

func (s *muxerSegmenter) writeH264(
	track *muxerTrack,
	ntp time.Time,
	pts int64,
	au [][]byte,
) error {
	randomAccess := false
	codec := track.Codec.(*codecs.H264)
	nonIDRPresent := false

	for _, nalu := range au {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeIDR:
			randomAccess = true

		case h264.NALUTypeNonIDR:
			nonIDRPresent = true

		case h264.NALUTypeSPS:
			if !bytes.Equal(codec.SPS, nalu) {
				s.pendingParamsChange = true
				codec.SPS = nalu
			}

		case h264.NALUTypePPS:
			if !bytes.Equal(codec.PPS, nalu) {
				s.pendingParamsChange = true
				codec.PPS = nalu
			}
		}
	}

	if !randomAccess && !nonIDRPresent {
		return nil
	}

	paramsChanged := false
	if randomAccess && s.pendingParamsChange {
		s.pendingParamsChange = false
		paramsChanged = true
	}

	// skip samples silently until we find a random access one
	if !track.firstRandomAccessReceived {
		if !randomAccess {
			return nil
		}
		track.firstRandomAccessReceived = true

		track.h264DTSExtractor = h264.NewDTSExtractor2()
	}

	dts, err := track.h264DTSExtractor.Extract(au, pts)
	if err != nil {
		return fmt.Errorf("unable to extract DTS: %w", err)
	}

	if s.muxer.Variant == MuxerVariantMPEGTS {
		if track.stream.nextSegment == nil {
			err := s.muxer.createFirstSegment(timestampToDuration(dts, track.ClockRate), ntp)
			if err != nil {
				return err
			}
		} else {
			// switch segment
			if randomAccess &&
				((timestampToDuration(dts, track.ClockRate)-track.stream.nextSegment.(*muxerSegmentMPEGTS).startDTS) >= s.muxer.SegmentMinDuration ||
					paramsChanged) {
				err := s.muxer.rotateSegments(timestampToDuration(dts, track.ClockRate), ntp, false)
				if err != nil {
					return err
				}
			}
		}

		err := track.stream.nextSegment.(*muxerSegmentMPEGTS).writeH264(
			track,
			pts,
			dts,
			randomAccess,
			au,
		)
		if err != nil {
			return err
		}

		return nil
	} else {
		ps, err := fmp4.NewPartSampleH26x(
			int32(pts-dts),
			randomAccess,
			au)
		if err != nil {
			return err
		}

		return s.fmp4WriteSample(
			track,
			randomAccess,
			paramsChanged,
			&fmp4AugmentedSample{
				PartSample: *ps,
				dts:        dts,
				ntp:        ntp,
			})
	}
}

func (s *muxerSegmenter) writeOpus(
	track *muxerTrack,
	ntp time.Time,
	pts int64,
	packets [][]byte,
) error {
	for _, packet := range packets {
		err := s.fmp4WriteSample(
			track,
			true,
			false,
			&fmp4AugmentedSample{
				PartSample: fmp4.PartSample{
					Payload: packet,
				},
				dts: pts,
				ntp: ntp,
			},
		)
		if err != nil {
			return err
		}

		duration := opus.PacketDuration(packet)
		ntp = ntp.Add(duration)
		pts += durationToTimestamp(duration, track.ClockRate)
	}

	return nil
}

func (s *muxerSegmenter) writeMPEG4Audio(
	track *muxerTrack,
	ntp time.Time,
	pts int64,
	aus [][]byte,
) error {
	if s.muxer.Variant == MuxerVariantMPEGTS {
		if track.isLeading {
			if track.stream.nextSegment == nil {
				err := s.muxer.createFirstSegment(timestampToDuration(pts, track.ClockRate), ntp)
				if err != nil {
					return err
				}
			} else if track.stream.nextSegment.(*muxerSegmentMPEGTS).audioAUCount >= mpegtsSegmentMinAUCount && // switch segment
				(timestampToDuration(pts, track.ClockRate)-track.stream.nextSegment.(*muxerSegmentMPEGTS).startDTS) >= s.muxer.SegmentMinDuration {
				err := s.muxer.rotateSegments(timestampToDuration(pts, track.ClockRate), ntp, false)
				if err != nil {
					return err
				}
			}
		} else {
			// wait for the video track
			if track.stream.nextSegment == nil {
				return nil
			}
		}

		err := track.stream.nextSegment.(*muxerSegmentMPEGTS).writeMPEG4Audio(track, pts, aus)
		if err != nil {
			return err
		}

		return nil
	} else {
		sampleRate := track.Codec.(*codecs.MPEG4Audio).Config.SampleRate

		for i, au := range aus {
			auNTP := ntp.Add(time.Duration(i) * mpeg4audio.SamplesPerAccessUnit *
				time.Second / time.Duration(sampleRate))
			auPTS := pts + int64(i)*mpeg4audio.SamplesPerAccessUnit*
				int64(track.ClockRate)/int64(sampleRate)

			err := s.fmp4WriteSample(
				track,
				true,
				false,
				&fmp4AugmentedSample{
					PartSample: fmp4.PartSample{
						Payload: au,
					},
					dts: auPTS,
					ntp: auNTP,
				},
			)
			if err != nil {
				return err
			}
		}

		return nil
	}
}

// iPhone iOS fails if part durations are less than 85% of maximum part duration.
// find a part duration that is compatible with all sample durations
func (s *muxerSegmenter) fmp4AdjustPartDuration(sampleDuration time.Duration) {
	if s.muxer.Variant != MuxerVariantLowLatency || s.fmp4FreezeAdjustedPartDuration {
		return
	}

	// avoid a crash by skipping invalid durations
	if sampleDuration == 0 {
		return
	}

	if _, ok := s.fmp4SampleDurations[sampleDuration]; !ok {
		s.fmp4SampleDurations[sampleDuration] = struct{}{}
		s.fmp4AdjustedPartDuration = findCompatiblePartDuration(
			s.muxer.PartMinDuration,
			s.fmp4SampleDurations,
		)
	}
}

func (s *muxerSegmenter) fmp4WriteSample(
	track *muxerTrack,
	randomAccess bool,
	paramsChanged bool,
	sample *fmp4AugmentedSample,
) error {
	// add a starting DTS to avoid a negative BaseTime
	sample.dts += durationToTimestamp(fmp4StartDTS, track.ClockRate)

	// BaseTime is still negative, this is not supported by fMP4. Reject the sample silently.
	if sample.dts < 0 {
		return nil
	}

	// put samples into a queue in order to compute the sample duration
	sample, track.fmp4NextSample = track.fmp4NextSample, sample
	if sample == nil {
		return nil
	}
	duration := track.fmp4NextSample.dts - sample.dts
	sample.Duration = uint32(duration)

	if track.isLeading {
		// create first segment
		if track.stream.nextSegment == nil {
			err := s.muxer.createFirstSegment(timestampToDuration(sample.dts, track.ClockRate), sample.ntp)
			if err != nil {
				return err
			}
		}
	} else {
		// wait for the leading track
		if track.stream.nextSegment == nil {
			return nil
		}
	}

	if track.isLeading {
		s.fmp4AdjustPartDuration(timestampToDuration(duration, track.ClockRate))
	}

	err := track.stream.nextSegment.(*muxerSegmentFMP4).writeSample(
		track,
		sample,
	)
	if err != nil {
		return err
	}

	if track.isLeading {
		// switch segment
		if randomAccess && (paramsChanged ||
			(timestampToDuration(track.fmp4NextSample.dts, track.ClockRate)-track.stream.nextSegment.(*muxerSegmentFMP4).startDTS) >= s.muxer.SegmentMinDuration) {
			err = s.muxer.rotateSegments(timestampToDuration(track.fmp4NextSample.dts, track.ClockRate), track.fmp4NextSample.ntp, paramsChanged)
			if err != nil {
				return err
			}

			// reset or freeze adjusted part duration
			if paramsChanged {
				s.fmp4FreezeAdjustedPartDuration = false
				s.fmp4SampleDurations = make(map[time.Duration]struct{})
			} else {
				s.fmp4FreezeAdjustedPartDuration = true
			}

			// switch part
		} else if (s.muxer.Variant == MuxerVariantLowLatency) &&
			(timestampToDuration(track.fmp4NextSample.dts, track.ClockRate)-track.stream.nextPart.startDTS) >= s.fmp4AdjustedPartDuration {
			err := s.muxer.rotateParts(timestampToDuration(track.fmp4NextSample.dts, track.ClockRate))
			if err != nil {
				return err
			}
		}
	}

	return nil
}
