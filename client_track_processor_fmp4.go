package gohlslib

import (
	"context"
	"fmt"
	"time"

	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
)

type clientTrackProcessorFMP4 struct {
	track                *Track
	onData               interface{}
	timeScale            uint32
	timeSync             *clientTimeSyncFMP4
	onPartTrackProcessed func(ctx context.Context)

	postProcess func(pts time.Duration, dts time.Duration, sample *fmp4.PartSample) error

	// in
	queue chan *fmp4.PartTrack
}

func (t *clientTrackProcessorFMP4) initialize() error {
	switch t.track.Codec.(type) {
	case *codecs.AV1:
		var onDataCasted ClientOnDataAV1Func = func(_ time.Duration, _ [][]byte) {}
		if t.onData != nil {
			onDataCasted = t.onData.(ClientOnDataAV1Func)
		}

		t.postProcess = func(pts time.Duration, dts time.Duration, sample *fmp4.PartSample) error {
			tu, err := sample.GetAV1()
			if err != nil {
				return err
			}

			onDataCasted(pts, tu)
			return nil
		}

	case *codecs.VP9:
		var onDataCasted ClientOnDataVP9Func = func(_ time.Duration, _ []byte) {}
		if t.onData != nil {
			onDataCasted = t.onData.(ClientOnDataVP9Func)
		}

		t.postProcess = func(pts time.Duration, dts time.Duration, sample *fmp4.PartSample) error {
			onDataCasted(pts, sample.Payload)
			return nil
		}

	case *codecs.H265, *codecs.H264:
		var onDataCasted ClientOnDataH26xFunc = func(_ time.Duration, _ time.Duration, _ [][]byte) {}
		if t.onData != nil {
			onDataCasted = t.onData.(ClientOnDataH26xFunc)
		}

		t.postProcess = func(pts time.Duration, dts time.Duration, sample *fmp4.PartSample) error {
			au, err := sample.GetH26x()
			if err != nil {
				return err
			}

			onDataCasted(pts, dts, au)
			return nil
		}

	case *codecs.Opus:
		var onDataCasted ClientOnDataOpusFunc = func(_ time.Duration, _ [][]byte) {}
		if t.onData != nil {
			onDataCasted = t.onData.(ClientOnDataOpusFunc)
		}

		t.postProcess = func(pts time.Duration, dts time.Duration, sample *fmp4.PartSample) error {
			onDataCasted(pts, [][]byte{sample.Payload})
			return nil
		}

	case *codecs.MPEG4Audio:
		var onDataCasted ClientOnDataMPEG4AudioFunc = func(_ time.Duration, _ [][]byte) {}
		if t.onData != nil {
			onDataCasted = t.onData.(ClientOnDataMPEG4AudioFunc)
		}

		t.postProcess = func(pts time.Duration, dts time.Duration, sample *fmp4.PartSample) error {
			onDataCasted(pts, [][]byte{sample.Payload})
			return nil
		}
	}

	t.queue = make(chan *fmp4.PartTrack)

	return nil
}

func (t *clientTrackProcessorFMP4) run(ctx context.Context) error {
	for {
		select {
		case partTrack := <-t.queue:
			err := t.process(ctx, partTrack)
			if err != nil {
				return err
			}

		case <-ctx.Done():
			return nil
		}
	}
}

func (t *clientTrackProcessorFMP4) process(ctx context.Context, partTrack *fmp4.PartTrack) error {
	rawDTS := partTrack.BaseTime

	for _, sample := range partTrack.Samples {
		pts := t.timeSync.convert(rawDTS+uint64(sample.PTSOffset), t.timeScale)
		dts := t.timeSync.convert(rawDTS, t.timeScale)
		rawDTS += uint64(sample.Duration)

		// silently discard packets prior to the first packet of the leading track
		if pts < 0 {
			continue
		}

		err := t.timeSync.sync(ctx, dts)
		if err != nil {
			return err
		}

		err = t.postProcess(pts, dts, sample)
		if err != nil {
			return err
		}
	}

	t.onPartTrackProcessed(ctx)
	return nil
}

func (t *clientTrackProcessorFMP4) push(ctx context.Context, partTrack *fmp4.PartTrack) error {
	select {
	case t.queue <- partTrack:
		return nil

	case <-ctx.Done():
		return fmt.Errorf("terminated")
	}
}
