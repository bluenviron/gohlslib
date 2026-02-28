package gohlslib

import (
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
)

type muxerTrack struct {
	*Track
	variant   MuxerVariant
	stream    *muxerStream
	isLeading bool

	firstRandomAccessReceived bool
	h264DTSExtractor          *h264.DTSExtractor
	h265DTSExtractor          *h265.DTSExtractor
	mpegtsTrack               *mpegts.Track        // mpegts only
	fmp4NextSample            *fmp4AugmentedSample // fmp4 only
	fmp4Samples               []*fmp4.Sample       // fmp4 only
	fmp4StartDTS              int64                // fmp4 only
}

func (t *muxerTrack) initialize() {
	if t.variant == MuxerVariantMPEGTS {
		t.mpegtsTrack = &mpegts.Track{
			Codec: toMPEGTS(t.Codec),
		}
	}
}
