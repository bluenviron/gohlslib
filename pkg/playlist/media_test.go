package playlist

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func playlistTypePtr(v MediaPlaylistType) *MediaPlaylistType {
	return &v
}

var casesMedia = []struct {
	name   string
	input  string
	output string
	dec    Media
}{
	{
		"gohlslib",
		"#EXTM3U\n" +
			"#EXT-X-VERSION:9\n" +
			"#EXT-X-INDEPENDENT-SEGMENTS\n" +
			"#EXT-X-ALLOW-CACHE:NO\n" +
			"#EXT-X-TARGETDURATION:8\n" +
			"#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=5.00000,CAN-SKIP-UNTIL=7\n" +
			"#EXT-X-PART-INF:PART-TARGET=2\n" +
			"#EXT-X-MEDIA-SEQUENCE:27\n" +
			"#EXT-X-MAP:URI=\"init.mp4\"\n" +
			"#EXT-X-SKIP:SKIPPED-SEGMENTS=15\n" +
			"#EXT-X-GAP\n" +
			"#EXTINF:2.00000,\n" +
			"gap.mp4\n" +
			"#EXT-X-PROGRAM-DATE-TIME:2014-08-25T00:00:00Z\n" +
			"#EXTINF:2.00000,\n" +
			"seg1.mp4\n" +
			"#EXT-X-PROGRAM-DATE-TIME:2014-08-25T00:00:00Z\n" +
			"#EXT-X-BITRATE:53314213213\n" +
			"#EXT-X-PART:DURATION=1.50000,URI=\"part1.mp4\",INDEPENDENT=YES\n" +
			"#EXT-X-PART:DURATION=1.50000,URI=\"part2.mp4\"\n" +
			"#EXTINF:3.00000,\n" +
			"seg2.mp4\n" +
			"#EXT-X-PART:DURATION=1.50000,URI=\"part3.mp4\",INDEPENDENT=YES\n" +
			"#EXT-X-PART:DURATION=1.50000,URI=\"part4.mp4\"\n" +
			"#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part5.mp4\",BYTERANGE-START=43523,BYTERANGE-LENGTH=123\n",
		"#EXTM3U\n" +
			"#EXT-X-VERSION:9\n" +
			"#EXT-X-INDEPENDENT-SEGMENTS\n" +
			"#EXT-X-ALLOW-CACHE:NO\n" +
			"#EXT-X-TARGETDURATION:8\n" +
			"#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=5.00000,CAN-SKIP-UNTIL=7\n" +
			"#EXT-X-PART-INF:PART-TARGET=2\n" +
			"#EXT-X-MEDIA-SEQUENCE:27\n" +
			"#EXT-X-MAP:URI=\"init.mp4\"\n" +
			"#EXT-X-SKIP:SKIPPED-SEGMENTS=15\n" +
			"#EXT-X-GAP\n" +
			"#EXTINF:2.00000,\n" +
			"gap.mp4\n" +
			"#EXT-X-PROGRAM-DATE-TIME:2014-08-25T00:00:00Z\n" +
			"#EXTINF:2.00000,\n" +
			"seg1.mp4\n" +
			"#EXT-X-PROGRAM-DATE-TIME:2014-08-25T00:00:00Z\n" +
			"#EXT-X-BITRATE:53314213213\n" +
			"#EXT-X-PART:DURATION=1.50000,URI=\"part1.mp4\",INDEPENDENT=YES\n" +
			"#EXT-X-PART:DURATION=1.50000,URI=\"part2.mp4\"\n" +
			"#EXTINF:3.00000,\n" +
			"seg2.mp4\n" +
			"#EXT-X-PART:DURATION=1.50000,URI=\"part3.mp4\",INDEPENDENT=YES\n" +
			"#EXT-X-PART:DURATION=1.50000,URI=\"part4.mp4\"\n" +
			"#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part5.mp4\",BYTERANGE-START=43523,BYTERANGE-LENGTH=123\n",
		Media{
			Version:             9,
			IndependentSegments: true,
			AllowCache:          boolPtr(false),
			TargetDuration:      8,
			ServerControl: &MediaServerControl{
				CanBlockReload: true,
				PartHoldBack:   durationPtr(5 * time.Second),
				CanSkipUntil:   durationPtr(7 * time.Second),
			},
			PartInf: &MediaPartInf{
				PartTarget: 2 * time.Second,
			},
			MediaSequence: 27,
			Map: &MediaMap{
				URI: "init.mp4",
			},
			Skip: &MediaSkip{
				SkippedSegments: 15,
			},
			Segments: []*MediaSegment{
				{
					Gap:      true,
					Duration: 2 * time.Second,
					URI:      "gap.mp4",
				},
				{
					DateTime: timePtr(time.Date(2014, 8, 25, 0, 0, 0, 0, time.UTC)),
					Duration: 2 * time.Second,
					URI:      "seg1.mp4",
				},
				{
					DateTime: timePtr(time.Date(2014, 8, 25, 0, 0, 0, 0, time.UTC)),
					Bitrate:  intPtr(53314213213),
					Duration: 3 * time.Second,
					URI:      "seg2.mp4",
					Parts: []*MediaPart{
						{
							Duration:    1500 * time.Millisecond,
							Independent: true,
							URI:         "part1.mp4",
						},
						{
							Duration: 1500 * time.Millisecond,
							URI:      "part2.mp4",
						},
					},
				},
			},
			Parts: []*MediaPart{
				{
					Duration:    1500 * time.Millisecond,
					Independent: true,
					URI:         "part3.mp4",
				},
				{
					Duration: 1500 * time.Millisecond,
					URI:      "part4.mp4",
				},
			},
			PreloadHint: &MediaPreloadHint{
				URI:             "part5.mp4",
				ByteRangeStart:  43523,
				ByteRangeLength: uint64Ptr(123),
			},
		},
	},
	{
		"apple vod",
		`#EXTM3U
#EXT-X-TARGETDURATION:6
#EXT-X-VERSION:7
#EXT-X-MEDIA-SEQUENCE:1
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-MAP:URI="main.mp4",BYTERANGE="721@0"
#EXTINF:6.00000,
#EXT-X-BYTERANGE:5874288@721
main.mp4
#EXTINF:6.00000,
#EXT-X-BYTERANGE:5863101@5875009
main.mp4
#EXTINF:6.00000,
#EXT-X-BYTERANGE:5856476@11738110
main.mp4
#EXTINF:6.00000,
#EXT-X-BYTERANGE:5859643@17594586
main.mp4
#EXT-X-ENDLIST
`,
		`#EXTM3U
#EXT-X-VERSION:7
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:1
#EXT-X-PLAYLIST-TYPE=VOD
#EXT-X-MAP:URI="main.mp4",BYTERANGE=721@0
#EXTINF:6.00000,
#EXT-X-BYTERANGE:5874288@721
main.mp4
#EXTINF:6.00000,
#EXT-X-BYTERANGE:5863101@5875009
main.mp4
#EXTINF:6.00000,
#EXT-X-BYTERANGE:5856476@11738110
main.mp4
#EXTINF:6.00000,
#EXT-X-BYTERANGE:5859643@17594586
main.mp4
#EXT-X-ENDLIST
`,
		Media{
			Version:             7,
			IndependentSegments: true,
			TargetDuration:      6,
			MediaSequence:       1,
			PlaylistType:        playlistTypePtr(MediaPlaylistTypeVOD),
			Map: &MediaMap{
				URI:             "main.mp4",
				ByteRangeLength: uint64Ptr(721),
				ByteRangeStart:  uint64Ptr(0),
			},
			Segments: []*MediaSegment{
				{
					Duration:        6 * time.Second,
					ByteRangeLength: uint64Ptr(5874288),
					ByteRangeStart:  uint64Ptr(721),
					URI:             "main.mp4",
				},
				{
					Duration:        6 * time.Second,
					ByteRangeLength: uint64Ptr(5863101),
					ByteRangeStart:  uint64Ptr(5875009),
					URI:             "main.mp4",
				},
				{
					Duration:        6 * time.Second,
					ByteRangeLength: uint64Ptr(5856476),
					ByteRangeStart:  uint64Ptr(11738110),
					URI:             "main.mp4",
				},
				{
					Duration:        6 * time.Second,
					ByteRangeLength: uint64Ptr(5859643),
					ByteRangeStart:  uint64Ptr(17594586),
					URI:             "main.mp4",
				},
			},
			Endlist: true,
		},
	},
}

func TestMediaUnmarshal(t *testing.T) {
	for _, ca := range casesMedia {
		t.Run(ca.name, func(t *testing.T) {
			var m Media
			err := m.Unmarshal([]byte(ca.input))
			require.NoError(t, err)
			require.Equal(t, ca.dec, m)
		})
	}
}

func TestMediaMarshal(t *testing.T) {
	for _, ca := range casesMedia {
		t.Run(ca.name, func(t *testing.T) {
			byts, err := ca.dec.Marshal()
			require.NoError(t, err)
			require.Equal(t, string(byts), ca.output)
		})
	}
}
