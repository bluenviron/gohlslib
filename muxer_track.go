package gohlslib

import (
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
)

type muxerTrack struct {
	*Track
	variant   MuxerVariant
	stream    *muxerStream
	isLeading bool

	firstRandomAccessReceived bool
	h264DTSExtractor          *h264.DTSExtractor2
	h265DTSExtractor          *h265.DTSExtractor2
	mpegtsTrack               *mpegts.Track        // mpegts only
	fmp4NextSample            *fmp4AugmentedSample // fmp4 only
	fmp4Samples               []*fmp4.PartSample   // fmp4 only
	fmp4StartDTS              int64                // fmp4 only
}

func (t *muxerTrack) initialize() {
	if t.variant == MuxerVariantMPEGTS {
		t.mpegtsTrack = &mpegts.Track{
			Codec: codecs.ToMPEGTS(t.Codec),
		}
	}
}
