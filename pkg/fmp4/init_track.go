package fmp4

import (
	gomp4 "github.com/abema/go-mp4"

	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"

	"github.com/bluenviron/gohlslib/pkg/codecs"
)

// InitTrack is a track of Init.
type InitTrack struct {
	ID        int
	TimeScale uint32
	Codec     codecs.Codec
}

func (track *InitTrack) marshal(w *mp4Writer) error {
	/*
		   trak
		   - tkhd
		   - mdia
			 - mdhd
			 - hdlr
			 - minf
			   - vmhd (video)
			   - smhd (audio)
			   - dinf
				 - dref
				   - url
			   - stbl
				 - stsd
				   - avc1 (h264)
					 - avcC
					 - btrt
				   - hev1 (h265)
					 - hvcC
				   - mp4a (mpeg4audio)
					 - esds
					 - btrt
				   - Opus (opus)
					 - dOps
					 - btrt
				 - stts
				 - stsc
				 - stsz
				 - stco
	*/

	_, err := w.writeBoxStart(&gomp4.Trak{}) // <trak>
	if err != nil {
		return err
	}

	var h264SPS []byte
	var h264PPS []byte
	var h264SPSP h264.SPS

	var h265VPS []byte
	var h265SPS []byte
	var h265PPS []byte
	var h265SPSP h265.SPS

	var width int
	var height int

	switch tcodec := track.Codec.(type) {
	case *codecs.H264:
		h264SPS, h264PPS = tcodec.SafeParams()

		err = h264SPSP.Unmarshal(h264SPS)
		if err != nil {
			return err
		}

		width = h264SPSP.Width()
		height = h264SPSP.Height()

	case *codecs.H265:
		h265VPS, h265SPS, h265PPS = tcodec.SafeParams()

		err = h265SPSP.Unmarshal(h265SPS)
		if err != nil {
			return err
		}

		width = h265SPSP.Width()
		height = h265SPSP.Height()
	}

	switch track.Codec.(type) {
	case *codecs.H264, *codecs.H265:
		_, err = w.writeBox(&gomp4.Tkhd{ // <tkhd/>
			FullBox: gomp4.FullBox{
				Flags: [3]byte{0, 0, 3},
			},
			TrackID: uint32(track.ID),
			Width:   uint32(width * 65536),
			Height:  uint32(height * 65536),
			Matrix:  [9]int32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000},
		})
		if err != nil {
			return err
		}

	case *codecs.MPEG4Audio, *codecs.Opus:
		_, err = w.writeBox(&gomp4.Tkhd{ // <tkhd/>
			FullBox: gomp4.FullBox{
				Flags: [3]byte{0, 0, 3},
			},
			TrackID:        uint32(track.ID),
			AlternateGroup: 1,
			Volume:         256,
			Matrix:         [9]int32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000},
		})
		if err != nil {
			return err
		}
	}

	_, err = w.writeBoxStart(&gomp4.Mdia{}) // <mdia>
	if err != nil {
		return err
	}

	_, err = w.writeBox(&gomp4.Mdhd{ // <mdhd/>
		Timescale: track.TimeScale,
		Language:  [3]byte{'u', 'n', 'd'},
	})
	if err != nil {
		return err
	}

	switch track.Codec.(type) {
	case *codecs.H264, *codecs.H265:
		_, err = w.writeBox(&gomp4.Hdlr{ // <hdlr/>
			HandlerType: [4]byte{'v', 'i', 'd', 'e'},
			Name:        "VideoHandler",
		})
		if err != nil {
			return err
		}

	case *codecs.MPEG4Audio, *codecs.Opus:
		_, err = w.writeBox(&gomp4.Hdlr{ // <hdlr/>
			HandlerType: [4]byte{'s', 'o', 'u', 'n'},
			Name:        "SoundHandler",
		})
		if err != nil {
			return err
		}
	}

	_, err = w.writeBoxStart(&gomp4.Minf{}) // <minf>
	if err != nil {
		return err
	}

	switch track.Codec.(type) {
	case *codecs.H264, *codecs.H265:
		_, err = w.writeBox(&gomp4.Vmhd{ // <vmhd/>
			FullBox: gomp4.FullBox{
				Flags: [3]byte{0, 0, 1},
			},
		})
		if err != nil {
			return err
		}

	case *codecs.MPEG4Audio, *codecs.Opus:
		_, err = w.writeBox(&gomp4.Smhd{ // <smhd/>
		})
		if err != nil {
			return err
		}
	}

	_, err = w.writeBoxStart(&gomp4.Dinf{}) // <dinf>
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&gomp4.Dref{ // <dref>
		EntryCount: 1,
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&gomp4.Url{ // <url/>
		FullBox: gomp4.FullBox{
			Flags: [3]byte{0, 0, 1},
		},
	})
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </dref>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </dinf>
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&gomp4.Stbl{}) // <stbl>
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&gomp4.Stsd{ // <stsd>
		EntryCount: 1,
	})
	if err != nil {
		return err
	}

	switch tcodec := track.Codec.(type) {
	case *codecs.H264:
		_, err = w.writeBoxStart(&gomp4.VisualSampleEntry{ // <avc1>
			SampleEntry: gomp4.SampleEntry{
				AnyTypeBox: gomp4.AnyTypeBox{
					Type: gomp4.BoxTypeAvc1(),
				},
				DataReferenceIndex: 1,
			},
			Width:           uint16(width),
			Height:          uint16(height),
			Horizresolution: 4718592,
			Vertresolution:  4718592,
			FrameCount:      1,
			Depth:           24,
			PreDefined3:     -1,
		})
		if err != nil {
			return err
		}

		_, err = w.writeBox(&gomp4.AVCDecoderConfiguration{ // <avcc/>
			AnyTypeBox: gomp4.AnyTypeBox{
				Type: gomp4.BoxTypeAvcC(),
			},
			ConfigurationVersion:       1,
			Profile:                    h264SPSP.ProfileIdc,
			ProfileCompatibility:       h264SPS[2],
			Level:                      h264SPSP.LevelIdc,
			LengthSizeMinusOne:         3,
			NumOfSequenceParameterSets: 1,
			SequenceParameterSets: []gomp4.AVCParameterSet{
				{
					Length:  uint16(len(h264SPS)),
					NALUnit: h264SPS,
				},
			},
			NumOfPictureParameterSets: 1,
			PictureParameterSets: []gomp4.AVCParameterSet{
				{
					Length:  uint16(len(h264PPS)),
					NALUnit: h264PPS,
				},
			},
		})
		if err != nil {
			return err
		}

		_, err = w.writeBox(&gomp4.Btrt{ // <btrt/>
			MaxBitrate: 1000000,
			AvgBitrate: 1000000,
		})
		if err != nil {
			return err
		}

		err = w.writeBoxEnd() // </avc1>
		if err != nil {
			return err
		}

	case *codecs.H265:
		_, err = w.writeBoxStart(&gomp4.VisualSampleEntry{ // <hev1>
			SampleEntry: gomp4.SampleEntry{
				AnyTypeBox: gomp4.AnyTypeBox{
					Type: gomp4.BoxTypeHev1(),
				},
				DataReferenceIndex: 1,
			},
			Width:           uint16(width),
			Height:          uint16(height),
			Horizresolution: 4718592,
			Vertresolution:  4718592,
			FrameCount:      1,
			Depth:           24,
			PreDefined3:     -1,
		})
		if err != nil {
			return err
		}

		_, err = w.writeBox(&gomp4.HvcC{ // <hvcC/>
			ConfigurationVersion:        1,
			GeneralProfileIdc:           h265SPSP.ProfileTierLevel.GeneralProfileIdc,
			GeneralProfileCompatibility: h265SPSP.ProfileTierLevel.GeneralProfileCompatibilityFlag,
			GeneralConstraintIndicator: [6]uint8{
				h265SPS[7], h265SPS[8], h265SPS[9],
				h265SPS[10], h265SPS[11], h265SPS[12],
			},
			GeneralLevelIdc: h265SPSP.ProfileTierLevel.GeneralLevelIdc,
			// MinSpatialSegmentationIdc
			// ParallelismType
			ChromaFormatIdc:      uint8(h265SPSP.ChromaFormatIdc),
			BitDepthLumaMinus8:   uint8(h265SPSP.BitDepthLumaMinus8),
			BitDepthChromaMinus8: uint8(h265SPSP.BitDepthChromaMinus8),
			// AvgFrameRate
			// ConstantFrameRate
			NumTemporalLayers: 1,
			// TemporalIdNested
			LengthSizeMinusOne: 3,
			NumOfNaluArrays:    3,
			NaluArrays: []gomp4.HEVCNaluArray{
				{
					NaluType: byte(h265.NALUType_VPS_NUT),
					NumNalus: 1,
					Nalus: []gomp4.HEVCNalu{{
						Length:  uint16(len(h265VPS)),
						NALUnit: h265VPS,
					}},
				},
				{
					NaluType: byte(h265.NALUType_SPS_NUT),
					NumNalus: 1,
					Nalus: []gomp4.HEVCNalu{{
						Length:  uint16(len(h265SPS)),
						NALUnit: h265SPS,
					}},
				},
				{
					NaluType: byte(h265.NALUType_PPS_NUT),
					NumNalus: 1,
					Nalus: []gomp4.HEVCNalu{{
						Length:  uint16(len(h265PPS)),
						NALUnit: h265PPS,
					}},
				},
			},
		})
		if err != nil {
			return err
		}

		_, err = w.writeBox(&gomp4.Btrt{ // <btrt/>
			MaxBitrate: 1000000,
			AvgBitrate: 1000000,
		})
		if err != nil {
			return err
		}

		err = w.writeBoxEnd() // </hev1>
		if err != nil {
			return err
		}

	case *codecs.MPEG4Audio:
		_, err = w.writeBoxStart(&gomp4.AudioSampleEntry{ // <mp4a>
			SampleEntry: gomp4.SampleEntry{
				AnyTypeBox: gomp4.AnyTypeBox{
					Type: gomp4.BoxTypeMp4a(),
				},
				DataReferenceIndex: 1,
			},
			ChannelCount: uint16(tcodec.ChannelCount),
			SampleSize:   16,
			SampleRate:   uint32(tcodec.SampleRate * 65536),
		})
		if err != nil {
			return err
		}

		enc, _ := tcodec.Config.Marshal()

		_, err = w.writeBox(&gomp4.Esds{ // <esds/>
			Descriptors: []gomp4.Descriptor{
				{
					Tag:  gomp4.ESDescrTag,
					Size: 32 + uint32(len(enc)),
					ESDescriptor: &gomp4.ESDescriptor{
						ESID: uint16(track.ID),
					},
				},
				{
					Tag:  gomp4.DecoderConfigDescrTag,
					Size: 18 + uint32(len(enc)),
					DecoderConfigDescriptor: &gomp4.DecoderConfigDescriptor{
						ObjectTypeIndication: 0x40,
						StreamType:           0x05,
						UpStream:             false,
						Reserved:             true,
						MaxBitrate:           128825,
						AvgBitrate:           128825,
					},
				},
				{
					Tag:  gomp4.DecSpecificInfoTag,
					Size: uint32(len(enc)),
					Data: enc,
				},
				{
					Tag:  gomp4.SLConfigDescrTag,
					Size: 1,
					Data: []byte{0x02},
				},
			},
		})
		if err != nil {
			return err
		}

		_, err = w.writeBox(&gomp4.Btrt{ // <btrt/>
			MaxBitrate: 128825,
			AvgBitrate: 128825,
		})
		if err != nil {
			return err
		}

		err = w.writeBoxEnd() // </mp4a>
		if err != nil {
			return err
		}

	case *codecs.Opus:
		_, err = w.writeBoxStart(&gomp4.AudioSampleEntry{ // <Opus>
			SampleEntry: gomp4.SampleEntry{
				AnyTypeBox: gomp4.AnyTypeBox{
					Type: BoxTypeOpus(),
				},
				DataReferenceIndex: 1,
			},
			ChannelCount: uint16(tcodec.Channels),
			SampleSize:   16,
			SampleRate:   48000 * 65536,
		})
		if err != nil {
			return err
		}

		_, err = w.writeBox(&DOps{ // <dOps/>
			OutputChannelCount: uint8(tcodec.Channels),
			PreSkip:            312,
			InputSampleRate:    48000,
		})
		if err != nil {
			return err
		}

		_, err = w.writeBox(&gomp4.Btrt{ // <btrt/>
			MaxBitrate: 128825,
			AvgBitrate: 128825,
		})
		if err != nil {
			return err
		}

		err = w.writeBoxEnd() // </Opus>
		if err != nil {
			return err
		}
	}

	err = w.writeBoxEnd() // </stsd>
	if err != nil {
		return err
	}

	_, err = w.writeBox(&gomp4.Stts{ // <stts>
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&gomp4.Stsc{ // <stsc>
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&gomp4.Stsz{ // <stsz>
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&gomp4.Stco{ // <stco>
	})
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </stbl>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </minf>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </mdia>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </trak>
	if err != nil {
		return err
	}

	return nil
}
