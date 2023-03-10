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
					for _, pd := range es.ElementaryStreamDescriptors {
						if pd.Registration != nil {
							if pd.Registration.FormatIdentifier == opusIdentifier {
								tracks = append(tracks, &Track{
									ES: es,
									Format: &format.Opus{
										ChannelCount: 2, // TODO: extract from 0x80
									},
								})
							}
						}
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
