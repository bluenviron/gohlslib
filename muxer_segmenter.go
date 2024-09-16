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

type muxerSegmenter struct {
	muxer *Muxer // TODO: remove

	pendingForceRotation           bool
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
	ntp time.Time,
	pts time.Duration,
	tu [][]byte,
) error {
	track := s.muxer.mtracksByTrack[s.muxer.VideoTrack]

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
				s.pendingForceRotation = true
				codec.SequenceHeader = obu
			}
		}
	}

	forceRotation := false
	if randomAccess && s.pendingForceRotation {
		s.pendingForceRotation = false
		forceRotation = true
	}

	if s.muxer.Variant == MuxerVariantMPEGTS {
		return fmt.Errorf("unimplemented")
	} else {
		ps, err := fmp4.NewPartSampleAV1(
			randomAccess,
			tu)
		if err != nil {
			return err
		}

		return s.fmp4WriteVideo(
			track,
			randomAccess,
			forceRotation,
			&fmp4AugmentedSample{
				PartSample: *ps,
				dts:        pts,
				ntp:        ntp,
			})
	}
}

func (s *muxerSegmenter) writeVP9(
	ntp time.Time,
	pts time.Duration,
	frame []byte,
) error {
	track := s.muxer.mtracksByTrack[s.muxer.VideoTrack]

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
			s.pendingForceRotation = true
			codec.Width = v
		}
		if v := h.Height(); v != codec.Height {
			s.pendingForceRotation = true
			codec.Height = v
		}
		if h.Profile != codec.Profile {
			s.pendingForceRotation = true
			codec.Profile = h.Profile
		}
		if h.ColorConfig.BitDepth != codec.BitDepth {
			s.pendingForceRotation = true
			codec.BitDepth = h.ColorConfig.BitDepth
		}
		if v := h.ChromaSubsampling(); v != codec.ChromaSubsampling {
			s.pendingForceRotation = true
			codec.ChromaSubsampling = v
		}
		if h.ColorConfig.ColorRange != codec.ColorRange {
			s.pendingForceRotation = true
			codec.ColorRange = h.ColorConfig.ColorRange
		}
	}

	forceRotation := false
	if randomAccess && s.pendingForceRotation {
		s.pendingForceRotation = false
		forceRotation = true
	}

	// skip samples silently until we find a random access one
	if !track.firstRandomAccessReceived {
		if !randomAccess {
			return nil
		}
		track.firstRandomAccessReceived = true
	}

	if s.muxer.Variant == MuxerVariantMPEGTS {
		return fmt.Errorf("unimplemented")
	} else {
		return s.fmp4WriteVideo(
			track,
			randomAccess,
			forceRotation,
			&fmp4AugmentedSample{
				PartSample: fmp4.PartSample{
					IsNonSyncSample: !randomAccess,
					Payload:         frame,
				},
				dts: pts,
				ntp: ntp,
			})
	}
}

func (s *muxerSegmenter) writeH265(
	ntp time.Time,
	pts time.Duration,
	au [][]byte,
) error {
	track := s.muxer.mtracksByTrack[s.muxer.VideoTrack]

	randomAccess := false
	codec := track.Codec.(*codecs.H265)

	for _, nalu := range au {
		typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case h265.NALUType_IDR_W_RADL, h265.NALUType_IDR_N_LP, h265.NALUType_CRA_NUT:
			randomAccess = true

		case h265.NALUType_VPS_NUT:
			if !bytes.Equal(codec.VPS, nalu) {
				s.pendingForceRotation = true
				codec.VPS = nalu
			}

		case h265.NALUType_SPS_NUT:
			if !bytes.Equal(codec.SPS, nalu) {
				s.pendingForceRotation = true
				codec.SPS = nalu
			}

		case h265.NALUType_PPS_NUT:
			if !bytes.Equal(codec.PPS, nalu) {
				s.pendingForceRotation = true
				codec.PPS = nalu
			}
		}
	}

	forceRotation := false
	if randomAccess && s.pendingForceRotation {
		s.pendingForceRotation = false
		forceRotation = true
	}

	// skip samples silently until we find a random access one
	if !track.firstRandomAccessReceived {
		if !randomAccess {
			return nil
		}
		track.firstRandomAccessReceived = true

		track.h265DTSExtractor = h265.NewDTSExtractor()
	}

	dts, err := track.h265DTSExtractor.Extract(au, pts)
	if err != nil {
		return fmt.Errorf("unable to extract DTS: %w", err)
	}

	if s.muxer.Variant == MuxerVariantMPEGTS {
		return fmt.Errorf("unimplemented")
	} else {
		ps, err := fmp4.NewPartSampleH26x(
			int32(durationGoToMp4(pts-dts, 90000)),
			randomAccess,
			au)
		if err != nil {
			return err
		}

		return s.fmp4WriteVideo(
			track,
			randomAccess,
			forceRotation,
			&fmp4AugmentedSample{
				PartSample: *ps,
				dts:        dts,
				ntp:        ntp,
			})
	}

}

func (s *muxerSegmenter) writeH264(
	ntp time.Time,
	pts time.Duration,
	au [][]byte,
) error {
	track := s.muxer.mtracksByTrack[s.muxer.VideoTrack]

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
				s.pendingForceRotation = true
				codec.SPS = nalu
			}

		case h264.NALUTypePPS:
			if !bytes.Equal(codec.PPS, nalu) {
				s.pendingForceRotation = true
				codec.PPS = nalu
			}
		}
	}

	if !randomAccess && !nonIDRPresent {
		return nil
	}

	forceRotation := false
	if randomAccess && s.pendingForceRotation {
		s.pendingForceRotation = false
		forceRotation = true
	}

	// skip samples silently until we find a random access one
	if !track.firstRandomAccessReceived {
		if !randomAccess {
			return nil
		}
		track.firstRandomAccessReceived = true

		track.h264DTSExtractor = h264.NewDTSExtractor()
	}

	dts, err := track.h264DTSExtractor.Extract(au, pts)
	if err != nil {
		return fmt.Errorf("unable to extract DTS: %w", err)
	}

	if s.muxer.Variant == MuxerVariantMPEGTS {
		if track.stream.nextSegment == nil {
			err := s.muxer.createFirstSegment(dts, ntp)
			if err != nil {
				return err
			}
		} else {
			// switch segment
			if randomAccess &&
				((dts-track.stream.nextSegment.(*muxerSegmentMPEGTS).startDTS) >= s.muxer.SegmentMinDuration ||
					forceRotation) {
				err := s.muxer.rotateSegments(dts, ntp, false)
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
			int32(durationGoToMp4(pts-dts, 90000)),
			randomAccess,
			au)
		if err != nil {
			return err
		}

		return s.fmp4WriteVideo(
			track,
			randomAccess,
			forceRotation,
			&fmp4AugmentedSample{
				PartSample: *ps,
				dts:        dts,
				ntp:        ntp,
			})
	}
}

func (s *muxerSegmenter) writeOpus(
	ntp time.Time,
	pts time.Duration,
	packets [][]byte,
) error {
	track := s.muxer.mtracksByTrack[s.muxer.AudioTrack]

	if s.muxer.Variant == MuxerVariantMPEGTS {
		return fmt.Errorf("unimplemented")
	} else {
		for _, packet := range packets {
			err := s.fmp4WriteAudio(
				track,
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
			pts += duration
		}

		return nil
	}
}

func (s *muxerSegmenter) writeMPEG4Audio(ntp time.Time, pts time.Duration, aus [][]byte) error {
	track := s.muxer.mtracksByTrack[s.muxer.AudioTrack]

	if s.muxer.Variant == MuxerVariantMPEGTS {
		if s.muxer.VideoTrack == nil {
			if track.stream.nextSegment == nil {
				err := s.muxer.createFirstSegment(pts, ntp)
				if err != nil {
					return err
				}
			} else if track.stream.nextSegment.(*muxerSegmentMPEGTS).audioAUCount >= mpegtsSegmentMinAUCount && // switch segment
				(pts-track.stream.nextSegment.(*muxerSegmentMPEGTS).startDTS) >= s.muxer.SegmentMinDuration {
				err := s.muxer.rotateSegments(pts, ntp, false)
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
		sampleRate := time.Duration(s.muxer.AudioTrack.Codec.(*codecs.MPEG4Audio).Config.SampleRate)

		for i, au := range aus {
			auNTP := ntp.Add(time.Duration(i) * mpeg4audio.SamplesPerAccessUnit *
				time.Second / sampleRate)
			auPTS := pts + time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*
				time.Second/sampleRate

			err := s.fmp4WriteAudio(
				track,
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
// find a part duration that is compatible with all received sample durations
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

func (s *muxerSegmenter) fmp4WriteVideo(
	track *muxerTrack,
	randomAccess bool,
	forceRotation bool,
	sample *fmp4AugmentedSample,
) error {
	// add a starting DTS to avoid a negative BaseTime
	sample.dts += fmp4StartDTS

	// BaseTime is still negative, this is not supported by fMP4. Reject the sample silently.
	if sample.dts < 0 {
		return nil
	}

	// put samples into a queue in order to
	// - compute sample duration
	// - check if next sample is IDR
	sample, track.fmp4NextSample = track.fmp4NextSample, sample
	if sample == nil {
		return nil
	}
	duration := track.fmp4NextSample.dts - sample.dts
	sample.Duration = uint32(durationGoToMp4(duration, 90000))

	if track.stream.nextSegment == nil {
		err := s.muxer.createFirstSegment(sample.dts, sample.ntp)
		if err != nil {
			return err
		}
	}

	s.fmp4AdjustPartDuration(duration)

	err := track.stream.nextSegment.(*muxerSegmentFMP4).writeSample(
		track,
		sample,
		track.fmp4NextSample.dts,
		s.fmp4AdjustedPartDuration)
	if err != nil {
		return err
	}

	// switch segment
	if randomAccess &&
		((track.fmp4NextSample.dts-track.stream.nextSegment.(*muxerSegmentFMP4).startDTS) >= s.muxer.SegmentMinDuration ||
			forceRotation) {
		err = s.muxer.rotateSegments(track.fmp4NextSample.dts, track.fmp4NextSample.ntp, forceRotation)
		if err != nil {
			return err
		}

		if forceRotation {
			// reset adjusted part duration
			s.fmp4FreezeAdjustedPartDuration = false
			s.fmp4SampleDurations = make(map[time.Duration]struct{})
		} else {
			s.fmp4FreezeAdjustedPartDuration = true
		}
	}

	return nil
}

func (s *muxerSegmenter) fmp4WriteAudio(
	track *muxerTrack,
	sample *fmp4AugmentedSample,
) error {
	// add a starting DTS to avoid a negative BaseTime
	sample.dts += fmp4StartDTS

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
	sample.Duration = uint32(durationGoToMp4(duration, track.fmp4TimeScale))

	if s.muxer.VideoTrack == nil {
		// create first segment
		if track.stream.nextSegment == nil {
			err := s.muxer.createFirstSegment(sample.dts, sample.ntp)
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

	err := track.stream.nextSegment.(*muxerSegmentFMP4).writeSample(
		track,
		sample,
		track.fmp4NextSample.dts,
		s.muxer.PartMinDuration)
	if err != nil {
		return err
	}

	// switch segment
	if s.muxer.VideoTrack == nil &&
		(track.fmp4NextSample.dts-track.stream.nextSegment.(*muxerSegmentFMP4).startDTS) >= s.muxer.SegmentMinDuration {
		err = s.muxer.rotateSegments(track.fmp4NextSample.dts, track.fmp4NextSample.ntp, false)
		if err != nil {
			return err
		}

		s.fmp4FreezeAdjustedPartDuration = true
	}

	return nil
}
