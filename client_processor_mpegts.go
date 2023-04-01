package gohlslib

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/asticode/go-astits"

	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"

	"github.com/bluenviron/gohlslib/pkg/codecs"
)

func mpegtsPickLeadingTrack(mpegtsTracks []*mpegts.Track) uint16 {
	// pick first video track
	for _, mt := range mpegtsTracks {
		if _, ok := mt.Codec.(*mpegts.CodecH264); ok {
			return mt.ES.ElementaryPID
		}
	}

	// otherwise, pick first track
	return mpegtsTracks[0].ES.ElementaryPID
}

func trackMPEGTSToHLS(mt *mpegts.Track) *Track {
	switch tcodec := mt.Codec.(type) {
	case *mpegts.CodecH264:
		return &Track{
			Codec: &codecs.H264{},
		}

	case *mpegts.CodecMPEG4Audio:
		return &Track{
			Codec: &codecs.MPEG4Audio{
				Config: tcodec.Config,
			},
		}
	}

	return nil
}

type clientProcessorMPEGTS struct {
	isLeading            bool
	segmentQueue         *clientSegmentQueue
	log                  LogFunc
	rp                   *clientRoutinePool
	onStreamTracks       func(context.Context, []*Track) bool
	onSetLeadingTimeSync func(clientTimeSync)
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool)
	onData               map[*Track]func(time.Duration, interface{})

	tracks          []*Track
	mpegtsTracks    []*mpegts.Track
	leadingTrackPID uint16
	trackProcs      map[uint16]*clientProcessorMPEGTSTrack
}

func newClientProcessorMPEGTS(
	isLeading bool,
	segmentQueue *clientSegmentQueue,
	log LogFunc,
	rp *clientRoutinePool,
	onStreamTracks func(context.Context, []*Track) bool,
	onSetLeadingTimeSync func(clientTimeSync),
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool),
	onData map[*Track]func(time.Duration, interface{}),
) *clientProcessorMPEGTS {
	return &clientProcessorMPEGTS{
		isLeading:            isLeading,
		segmentQueue:         segmentQueue,
		log:                  log,
		rp:                   rp,
		onStreamTracks:       onStreamTracks,
		onSetLeadingTimeSync: onSetLeadingTimeSync,
		onGetLeadingTimeSync: onGetLeadingTimeSync,
		onData:               onData,
	}
}

func (p *clientProcessorMPEGTS) run(ctx context.Context) error {
	for {
		seg, ok := p.segmentQueue.pull(ctx)
		if !ok {
			return fmt.Errorf("terminated")
		}

		err := p.processSegment(ctx, seg)
		if err != nil {
			return err
		}
	}
}

func (p *clientProcessorMPEGTS) processSegment(ctx context.Context, byts []byte) error {
	if p.mpegtsTracks == nil {
		dem := astits.NewDemuxer(context.Background(), bytes.NewReader(byts))

		var err error
		p.mpegtsTracks, err = mpegts.FindTracks(dem)
		if err != nil {
			return err
		}

		for _, track := range p.mpegtsTracks {
			switch track.Codec.(type) {
			case *mpegts.CodecH264, *mpegts.CodecMPEG4Audio:
			default:
				return fmt.Errorf("unsupported track type: %T", track)
			}
		}

		p.leadingTrackPID = mpegtsPickLeadingTrack(p.mpegtsTracks)

		p.tracks = make([]*Track, len(p.mpegtsTracks))
		for i, mt := range p.mpegtsTracks {
			p.tracks[i] = trackMPEGTSToHLS(mt)
		}

		ok := p.onStreamTracks(ctx, p.tracks)
		if !ok {
			return fmt.Errorf("terminated")
		}
	}

	dem := astits.NewDemuxer(context.Background(), bytes.NewReader(byts))

	for {
		data, err := dem.NextData()
		if err != nil {
			if err == astits.ErrNoMorePackets {
				return nil
			}
			if strings.HasPrefix(err.Error(), "astits: parsing PES data failed") {
				continue
			}
			return err
		}

		if data.PES == nil {
			continue
		}

		if data.PES.Header.OptionalHeader == nil ||
			data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorNoPTSOrDTS ||
			data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorIsForbidden {
			return fmt.Errorf("PTS is missing")
		}

		if p.trackProcs == nil {
			var ts *clientTimeSyncMPEGTS

			if p.isLeading {
				if data.PID != p.leadingTrackPID {
					continue
				}

				var dts int64
				if data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorBothPresent {
					dts = data.PES.Header.OptionalHeader.DTS.Base
				} else {
					dts = data.PES.Header.OptionalHeader.PTS.Base
				}

				ts = newClientTimeSyncMPEGTS(dts)
				p.onSetLeadingTimeSync(ts)
			} else {
				rawTS, ok := p.onGetLeadingTimeSync(ctx)
				if !ok {
					return fmt.Errorf("terminated")
				}

				ts, ok = rawTS.(*clientTimeSyncMPEGTS)
				if !ok {
					return fmt.Errorf("stream playlists are mixed MPEGTS/FMP4")
				}
			}

			p.initializeTrackProcs(ts)
		}

		proc, ok := p.trackProcs[data.PID]
		if !ok {
			continue
		}

		select {
		case proc.queue <- data.PES:
		case <-ctx.Done():
		}
	}
}

func (p *clientProcessorMPEGTS) initializeTrackProcs(ts *clientTimeSyncMPEGTS) {
	p.trackProcs = make(map[uint16]*clientProcessorMPEGTSTrack)

	for i, track := range p.tracks {
		var cb func(time.Duration, []byte) error

		cb2, ok := p.onData[track]
		if !ok {
			cb2 = func(time.Duration, interface{}) {
			}
		}

		switch track.Codec.(type) {
		case *codecs.H264:
			cb = func(pts time.Duration, payload []byte) error {
				nalus, err := h264.AnnexBUnmarshal(payload)
				if err != nil {
					p.log(LogLevelWarn, "unable to decode Annex-B: %s", err)
					return nil
				}

				cb2(pts, nalus)
				return nil
			}

		case *codecs.MPEG4Audio:
			cb = func(pts time.Duration, payload []byte) error {
				var adtsPkts mpeg4audio.ADTSPackets
				err := adtsPkts.Unmarshal(payload)
				if err != nil {
					return fmt.Errorf("unable to decode ADTS: %s", err)
				}

				for i, pkt := range adtsPkts {
					cb2(
						pts+time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*time.Second/time.Duration(pkt.SampleRate),
						pkt.AU)
				}

				return nil
			}
		}

		proc := newClientProcessorMPEGTSTrack(
			ts,
			cb,
		)
		p.rp.add(proc)
		p.trackProcs[p.mpegtsTracks[i].ES.ElementaryPID] = proc
	}
}
