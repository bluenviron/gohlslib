package mpegts

import (
	"fmt"

	"github.com/aler9/gortsplib/v2/pkg/codecs/mpeg4audio"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/asticode/go-astits"
)

const (
	opusIdentifier = uint32('O')<<24 | uint32('p')<<16 | uint32('u')<<8 | uint32('s')
)

func findMPEG4AudioConfig(dem *astits.Demuxer, pid uint16) (*mpeg4audio.Config, error) {
	for {
		data, err := dem.NextData()
		if err != nil {
			return nil, err
		}

		if data.PES == nil || data.PID != pid {
			continue
		}

		var adtsPkts mpeg4audio.ADTSPackets
		err = adtsPkts.Unmarshal(data.PES.Data)
		if err != nil {
			return nil, fmt.Errorf("unable to decode ADTS: %s", err)
		}

		pkt := adtsPkts[0]
		return &mpeg4audio.Config{
			Type:         pkt.Type,
			SampleRate:   pkt.SampleRate,
			ChannelCount: pkt.ChannelCount,
		}, nil
	}
}

func findOpusRegistration(descriptors []*astits.Descriptor) bool {
	for _, sd := range descriptors {
		if sd.Registration != nil {
			if sd.Registration.FormatIdentifier == opusIdentifier {
				return true
			}
		}
	}
	return false
}

func findOpusChannelCount(descriptors []*astits.Descriptor) int {
	for _, sd := range descriptors {
		if sd.Extension != nil && sd.Extension.Tag == 0x80 &&
			sd.Extension.Unknown != nil && len(*sd.Extension.Unknown) >= 1 {
			return int((*sd.Extension.Unknown)[0])
		}
	}
	return 0
}

func findOpusTrack(descriptors []*astits.Descriptor) *format.Opus {
	if !findOpusRegistration(descriptors) {
		return nil
	}

	channelCount := findOpusChannelCount(descriptors)
	if channelCount <= 0 {
		return nil
	}

	return &format.Opus{
		PayloadTyp: 96,
		IsStereo:   (channelCount == 2),
	}
}

// Track is a MPEG-TS track.
type Track struct {
	ES     *astits.PMTElementaryStream
	Format format.Format
}

// FindTracks finds the tracks in a MPEG-TS stream.
func FindTracks(dem *astits.Demuxer) ([]*Track, error) {
	var tracks []*Track

	for {
		data, err := dem.NextData()
		if err != nil {
			return nil, err
		}

		if data.PMT != nil {
			for _, es := range data.PMT.ElementaryStreams {
				switch es.StreamType {
				case astits.StreamTypeH264Video:
					tracks = append(tracks, &Track{
						ES: es,
						Format: &format.H264{
							PayloadTyp:        96,
							PacketizationMode: 1,
						},
					})

				case astits.StreamTypeH265Video:
					tracks = append(tracks, &Track{
						ES: es,
						Format: &format.H265{
							PayloadTyp: 96,
						},
					})

				case astits.StreamTypeAACAudio:
					conf, err := findMPEG4AudioConfig(dem, es.ElementaryPID)
					if err != nil {
						return nil, err
					}

					tracks = append(tracks, &Track{
						ES: es,
						Format: &format.MPEG4Audio{
							PayloadTyp:       96,
							Config:           conf,
							SizeLength:       13,
							IndexLength:      3,
							IndexDeltaLength: 3,
						},
					})

				case astits.StreamTypePrivateData:
					format := findOpusTrack(es.ElementaryStreamDescriptors)
					if format != nil {
						tracks = append(tracks, &Track{
							ES:     es,
							Format: format,
						})
					}
				}
			}
			break
		}
	}

	if tracks == nil {
		return nil, fmt.Errorf("no tracks found")
	}

	return tracks, nil
}
