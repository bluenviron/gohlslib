package gohlslib

import (
	"fmt"
	"io"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"

	"github.com/bluenviron/gohlslib/v2/pkg/storage"
)

type muxerPart struct {
	segmentMaxSize uint64
	streamID       string
	streamTracks   []*muxerTrack
	segment        *muxerSegmentFMP4
	startDTS       time.Duration
	prefix         string
	id             uint64
	storage        storage.Part

	path          string
	isIndependent bool
	endDTS        time.Duration // available after finalize()
}

func (p *muxerPart) initialize() {
	p.path = partPath(p.prefix, p.streamID, p.id)
}

func (p *muxerPart) reader() (io.ReadCloser, error) {
	return p.storage.Reader()
}

func (p *muxerPart) getDuration() time.Duration {
	return p.endDTS - p.startDTS
}

func (p *muxerPart) finalize(endDTS time.Duration) error {
	part := fmp4.Part{
		SequenceNumber: uint32(p.id),
	}

	p.isIndependent = false

	for i, track := range p.streamTracks {
		// samples that start after the end of the part are moved into the next part
		samples := track.fmp4Samples
		var nextSamples []*fmp4.Sample
		startDTS := track.fmp4StartDTS
		dts := startDTS

		if endDTS != 0 {
			for j, sample := range samples {
				if timestampToDuration(dts, track.ClockRate) >= endDTS {
					nextSamples = samples[j:]
					samples = samples[:j]
					break
				}
				dts += int64(sample.Duration)
			}

			// the queued sample of a non-leading track can start before the end of the part.
			// if its duration is known in advance, write it instead of leaving the part empty.
			if len(samples) == 0 && !track.isLeading && track.fmp4NextSample != nil &&
				track.fmp4NextSample.duration != 0 &&
				timestampToDuration(track.fmp4NextSample.dts, track.ClockRate) < endDTS &&
				(p.segment.size+uint64(len(track.fmp4NextSample.Payload))) <= p.segmentMaxSize {
				sample := track.fmp4NextSample
				p.segment.size += uint64(len(sample.Payload))
				track.fmp4NextSample = nil
				sample.Duration = uint32(sample.duration)
				samples = []*fmp4.Sample{&sample.Sample}
				startDTS = sample.dts
				dts = sample.dts + int64(sample.Duration)
			}
		}

		if len(samples) != 0 {
			part.Tracks = append(part.Tracks, &fmp4.PartTrack{
				ID:       1 + i,
				BaseTime: uint64(startDTS),
				Samples:  samples,
			})

			if track.isLeading || len(track.stream.tracks) == 1 {
				for _, sample := range samples {
					if !sample.IsNonSyncSample {
						p.isIndependent = true
						break
					}
				}
			}
		}

		track.fmp4Samples = nextSamples
		track.fmp4StartDTS = dts
	}

	err := part.Marshal(p.storage.Writer())
	if err != nil {
		return err
	}

	p.endDTS = endDTS

	return nil
}

func (p *muxerPart) writeSample(track *muxerTrack, sample *fmp4AugmentedSample) error {
	size := uint64(len(sample.Payload))
	if (p.segment.size + size) > p.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	p.segment.size += size

	if track.fmp4Samples == nil {
		track.fmp4StartDTS = sample.dts
	}

	track.fmp4Samples = append(track.fmp4Samples, &sample.Sample)

	return nil
}
