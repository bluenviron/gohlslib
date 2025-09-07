package playlist

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func ptrOf[T any](v T) *T {
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
			"#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=5.00000,CAN-SKIP-UNTIL=7.00000\n" +
			"#EXT-X-PART-INF:PART-TARGET=2.00000\n" +
			"#EXT-X-MEDIA-SEQUENCE:27\n" +
			"#EXT-X-MAP:URI=\"init.mp4\"\n" +
			"#EXT-X-SKIP:SKIPPED-SEGMENTS=15\n" +
			"#EXT-X-GAP\n" +
			"#EXTINF:2.00000,\n" +
			"gap.mp4\n" +
			"#EXT-X-PROGRAM-DATE-TIME:2014-08-25T00:00:00Z\n" +
			"#EXTINF:2.00000,\n" +
			"seg1.mp4\n" +
			"#EXT-X-DISCONTINUITY\n" +
			"#EXT-X-PROGRAM-DATE-TIME:2014-08-25T00:00:00Z\n" +
			"#EXT-X-BITRATE:14213213\n" +
			"#EXT-X-PART:DURATION=1.50000,URI=\"part1.mp4\",INDEPENDENT=YES\n" +
			"#EXT-X-PART:DURATION=1.50000,URI=\"part2.mp4\",BYTERANGE=456@123\n" +
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
			"#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=5.00000,CAN-SKIP-UNTIL=7.00000\n" +
			"#EXT-X-PART-INF:PART-TARGET=2.00000\n" +
			"#EXT-X-MEDIA-SEQUENCE:27\n" +
			"#EXT-X-MAP:URI=\"init.mp4\"\n" +
			"#EXT-X-SKIP:SKIPPED-SEGMENTS=15\n" +
			"#EXT-X-GAP\n" +
			"#EXTINF:2.00000,\n" +
			"gap.mp4\n" +
			"#EXT-X-PROGRAM-DATE-TIME:2014-08-25T00:00:00Z\n" +
			"#EXTINF:2.00000,\n" +
			"seg1.mp4\n" +
			"#EXT-X-DISCONTINUITY\n" +
			"#EXT-X-PROGRAM-DATE-TIME:2014-08-25T00:00:00Z\n" +
			"#EXT-X-BITRATE:14213213\n" +
			"#EXT-X-PART:DURATION=1.50000,URI=\"part1.mp4\",INDEPENDENT=YES\n" +
			"#EXT-X-PART:DURATION=1.50000,URI=\"part2.mp4\",BYTERANGE=456@123\n" +
			"#EXTINF:3.00000,\n" +
			"seg2.mp4\n" +
			"#EXT-X-PART:DURATION=1.50000,URI=\"part3.mp4\",INDEPENDENT=YES\n" +
			"#EXT-X-PART:DURATION=1.50000,URI=\"part4.mp4\"\n" +
			"#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part5.mp4\",BYTERANGE-START=43523,BYTERANGE-LENGTH=123\n",
		Media{
			Version:             9,
			IndependentSegments: true,
			AllowCache:          ptrOf(false),
			TargetDuration:      8,
			ServerControl: &MediaServerControl{
				CanBlockReload: true,
				PartHoldBack:   ptrOf(5 * time.Second),
				CanSkipUntil:   ptrOf(7 * time.Second),
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
					DateTime: ptrOf(time.Date(2014, 8, 25, 0, 0, 0, 0, time.UTC)),
					Duration: 2 * time.Second,
					URI:      "seg1.mp4",
				},
				{
					DateTime:      ptrOf(time.Date(2014, 8, 25, 0, 0, 0, 0, time.UTC)),
					Bitrate:       ptrOf(14213213),
					Duration:      3 * time.Second,
					URI:           "seg2.mp4",
					Discontinuity: true,
					Parts: []*MediaPart{
						{
							Duration:    1500 * time.Millisecond,
							Independent: true,
							URI:         "part1.mp4",
						},
						{
							Duration:        1500 * time.Millisecond,
							URI:             "part2.mp4",
							ByteRangeLength: ptrOf(uint64(456)),
							ByteRangeStart:  ptrOf(uint64(123)),
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
				ByteRangeLength: ptrOf(uint64(123)),
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
#EXT-X-PLAYLIST-TYPE:VOD
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
			PlaylistType:        ptrOf(MediaPlaylistTypeVOD),
			Map: &MediaMap{
				URI:             "main.mp4",
				ByteRangeLength: ptrOf(uint64(721)),
				ByteRangeStart:  ptrOf(uint64(0)),
			},
			Segments: []*MediaSegment{
				{
					Duration:        6 * time.Second,
					ByteRangeLength: ptrOf(uint64(5874288)),
					ByteRangeStart:  ptrOf(uint64(721)),
					URI:             "main.mp4",
				},
				{
					Duration:        6 * time.Second,
					ByteRangeLength: ptrOf(uint64(5863101)),
					ByteRangeStart:  ptrOf(uint64(5875009)),
					URI:             "main.mp4",
				},
				{
					Duration:        6 * time.Second,
					ByteRangeLength: ptrOf(uint64(5856476)),
					ByteRangeStart:  ptrOf(uint64(11738110)),
					URI:             "main.mp4",
				},
				{
					Duration:        6 * time.Second,
					ByteRangeLength: ptrOf(uint64(5859643)),
					ByteRangeStart:  ptrOf(uint64(17594586)),
					URI:             "main.mp4",
				},
			},
			Endlist: true,
		},
	},
	{
		"key-basic",
		`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=AES-128,URI="key.bin"
#EXTINF:6.00000,
segment1.ts
#EXTINF:6.00000,
segment2.ts`,
		`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=AES-128,URI="key.bin"
#EXTINF:6.00000,
segment1.ts
#EXTINF:6.00000,
segment2.ts
`,
		Media{
			Version:        3,
			TargetDuration: 6,
			Segments: []*MediaSegment{
				{
					Duration: 6 * time.Second,
					URI:      "segment1.ts",
					Key: &MediaKey{
						Method: MediaKeyMethodAES128,
						URI:    "key.bin",
					},
				},
				{
					Duration: 6 * time.Second,
					URI:      "segment2.ts",
					Key: &MediaKey{
						Method: MediaKeyMethodAES128,
						URI:    "key.bin",
					},
				},
			},
		},
	},
	{
		"key-with-iv",
		`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=AES-128,URI="key.bin",IV=0x1234567890abcdef1234567890abcdef
#EXTINF:6.00000,
segment1.ts
`,
		`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=AES-128,URI="key.bin",IV=0x1234567890abcdef1234567890abcdef
#EXTINF:6.00000,
segment1.ts
`,
		Media{
			Version:        3,
			TargetDuration: 6,
			Segments: []*MediaSegment{
				{
					Duration: 6 * time.Second,
					URI:      "segment1.ts",
					Key: &MediaKey{
						Method: MediaKeyMethodAES128,
						URI:    "key.bin",
						IV:     "0x1234567890abcdef1234567890abcdef",
					},
				},
			},
		},
	},
	{
		"key-with-format",
		`#EXTM3U
#EXT-X-VERSION:5
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=SAMPLE-AES,URI="key.bin",KEYFORMAT="com.apple.streamingkeydelivery",KEYFORMATVERSIONS="1"
#EXTINF:6.00000,
segment1.ts
`,
		`#EXTM3U
#EXT-X-VERSION:5
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=SAMPLE-AES,URI="key.bin",KEYFORMAT="com.apple.streamingkeydelivery",KEYFORMATVERSIONS="1"
#EXTINF:6.00000,
segment1.ts
`,
		Media{
			Version:        5,
			TargetDuration: 6,
			Segments: []*MediaSegment{
				{
					Duration: 6 * time.Second,
					URI:      "segment1.ts",
					Key: &MediaKey{
						Method:            MediaKeyMethodSampleAES,
						URI:               "key.bin",
						KeyFormat:         "com.apple.streamingkeydelivery",
						KeyFormatVersions: "1",
					},
				},
			},
		},
	},
	{
		"key-none",
		`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=NONE
#EXTINF:6.00000,
segment1.ts`,
		`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=NONE
#EXTINF:6.00000,
segment1.ts
`,
		Media{
			Version:        3,
			TargetDuration: 6,
			Segments: []*MediaSegment{
				{
					Duration: 6 * time.Second,
					URI:      "segment1.ts",
					Key: &MediaKey{
						Method: MediaKeyMethodNone,
					},
				},
			},
		},
	},
	{
		"missing extinf comma",
		`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:4
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=NONE
#EXTINF:4.00000
segment1.ts`,
		`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:4
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=NONE
#EXTINF:4.00000,
segment1.ts
`,
		Media{
			Version:        3,
			TargetDuration: 4,
			Segments: []*MediaSegment{
				{
					Duration: 4 * time.Second,
					URI:      "segment1.ts",
					Key: &MediaKey{
						Method: MediaKeyMethodNone,
					},
				},
			},
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

func TestMediaUnmarshalDecimalTargetDuration(t *testing.T) {
	enc := "#EXTM3U\n" +
		"#EXT-X-VERSION:9\n" +
		"#EXT-X-TARGETDURATION:2.0000\n" +
		"#EXTINF:2.00000,\n" +
		"seg.mp4\n"

	var m Media
	err := m.Unmarshal([]byte(enc))
	require.NoError(t, err)
	require.Equal(t, m.TargetDuration, 2)
}

func TestMediaUnmarshalMissingTrailingNewline(t *testing.T) {
	enc := "#EXTM3U\n" +
		"#EXT-X-VERSION:9\n" +
		"#EXT-X-TARGETDURATION:2.0000\n" +
		"#EXTINF:2.00000,\n" +
		"seg.mp4\n" +
		"#EXT-X-ENDLIST"

	var m Media
	err := m.Unmarshal([]byte(enc))
	require.NoError(t, err)
	require.Equal(t, true, m.Endlist)
}

func TestMediaUnmarshalDateTime(t *testing.T) {
	for _, ca := range []struct {
		name     string
		enc      string
		dateTime time.Time
	}{
		{
			"iso8601",
			"#EXTM3U\n" +
				"#EXT-X-VERSION:9\n" +
				"#EXT-X-TARGETDURATION:8\n" +
				"#EXT-X-PROGRAM-DATE-TIME:2023-06-16T21:08:02.686-0400\n" +
				"#EXTINF:2.00000,\n" +
				"seg.mp4\n",
			time.Date(2023, 6, 17, 1, 8, 2, 686000000, time.UTC),
		},
		{
			"rfc3336",
			"#EXTM3U\n" +
				"#EXT-X-VERSION:9\n" +
				"#EXT-X-TARGETDURATION:8\n" +
				"#EXT-X-PROGRAM-DATE-TIME:2014-08-25T00:00:00-04:00\n" +
				"#EXTINF:2.00000,\n" +
				"seg.mp4\n",
			time.Date(2014, 8, 25, 4, 0, 0, 0, time.UTC),
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			var m Media
			err := m.Unmarshal([]byte(ca.enc))
			require.NoError(t, err)
			require.Equal(t, ca.dateTime, m.Segments[0].DateTime.UTC())
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

func FuzzMediaUnmarshal(f *testing.F) {
	for _, ca := range casesMedia {
		f.Add(ca.input)
	}

	f.Add("#EXTM3U\n" +
		"#EXT-X-PART:DURATION=1.50000,URI=\"part3.mp4\",INDEPENDENT=YES,BYTERANGE=\n")

	f.Fuzz(func(t *testing.T, a string) {
		var m Media
		err := m.Unmarshal([]byte(a))
		if err != nil {
			return
		}

		_, err = m.Marshal()
		require.NoError(t, err)
	})
}
