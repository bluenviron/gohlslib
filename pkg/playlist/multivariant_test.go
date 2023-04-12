//nolint:lll
package playlist

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int {
	return &v
}

func floatPtr(v float64) *float64 {
	return &v
}

func boolPtr(v bool) *bool {
	return &v
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}

func durationPtr(v time.Duration) *time.Duration {
	return &v
}

func timePtr(v time.Time) *time.Time {
	return &v
}

var casesMultivariant = []struct {
	name   string
	input  string
	output string
	dec    Multivariant
}{
	{
		"gohlslib",
		"#EXTM3U\n" +
			"#EXT-X-VERSION:9\n" +
			"#EXT-X-INDEPENDENT-SEGMENTS\n" +
			"#EXT-X-START:TIME-OFFSET=15.00000\n" +
			"\n" +
			"#EXT-X-STREAM-INF:BANDWIDTH=155000,AVERAGE-BANDWIDTH=120000,CODECS=\"avc1.42c028,mp4a.40.2\"" +
			",RESOLUTION=1280x720,FRAME-RATE=24.000,AUDIO=\"aud1\",SUBTITLES=\"sub1\"\n" +
			"stream1.m3u8\n" +
			"#EXT-X-STREAM-INF:BANDWIDTH=55000,AVERAGE-BANDWIDTH=20000,CODECS=\"avc1.42c028,mp4a.40.2\"" +
			",RESOLUTION=1280x720,FRAME-RATE=24.000\n" +
			"stream2.m3u8\n" +
			"\n" +
			"#EXT-X-MEDIA:TYPE=\"AUDIO\",GROUP-ID=\"aud1\",LANGUAGE=\"en\",NAME=\"english\"" +
			",DEFAULT=YES,AUTOSELECT=YES,CHANNELS=\"2\",URI=\"audio.m3u8\"\n" +
			"#EXT-X-MEDIA:TYPE=\"SUBTITLES\",GROUP-ID=\"sub1\",LANGUAGE=\"en\",NAME=\"english\"" +
			",DEFAULT=YES,AUTOSELECT=YES,FORCED=NO,URI=\"sub.m3u8\"\n",
		"#EXTM3U\n" +
			"#EXT-X-VERSION:9\n" +
			"#EXT-X-INDEPENDENT-SEGMENTS\n" +
			"#EXT-X-START:TIME-OFFSET=15.00000\n" +
			"\n" +
			"#EXT-X-STREAM-INF:BANDWIDTH=155000,AVERAGE-BANDWIDTH=120000,CODECS=\"avc1.42c028,mp4a.40.2\"" +
			",RESOLUTION=1280x720,FRAME-RATE=24.000,AUDIO=\"aud1\",SUBTITLES=\"sub1\"\n" +
			"stream1.m3u8\n" +
			"#EXT-X-STREAM-INF:BANDWIDTH=55000,AVERAGE-BANDWIDTH=20000,CODECS=\"avc1.42c028,mp4a.40.2\"" +
			",RESOLUTION=1280x720,FRAME-RATE=24.000\n" +
			"stream2.m3u8\n" +
			"\n" +
			"#EXT-X-MEDIA:TYPE=\"AUDIO\",GROUP-ID=\"aud1\",LANGUAGE=\"en\",NAME=\"english\"" +
			",DEFAULT=YES,AUTOSELECT=YES,CHANNELS=\"2\",URI=\"audio.m3u8\"\n" +
			"#EXT-X-MEDIA:TYPE=\"SUBTITLES\",GROUP-ID=\"sub1\",LANGUAGE=\"en\",NAME=\"english\"" +
			",DEFAULT=YES,AUTOSELECT=YES,FORCED=NO,URI=\"sub.m3u8\"\n",
		Multivariant{
			Version:             9,
			IndependentSegments: true,
			Start: &MultivariantStart{
				TimeOffset: 15 * time.Second,
			},
			Variants: []*MultivariantVariant{
				{
					Bandwidth:        155000,
					AverageBandwidth: intPtr(120000),
					Codecs: []string{
						"avc1.42c028",
						"mp4a.40.2",
					},
					Resolution: "1280x720",
					FrameRate:  floatPtr(24.0),
					Audio:      "aud1",
					Subtitles:  "sub1",
					URI:        "stream1.m3u8",
				},
				{
					Bandwidth:        55000,
					AverageBandwidth: intPtr(20000),
					Codecs: []string{
						"avc1.42c028",
						"mp4a.40.2",
					},
					Resolution: "1280x720",
					FrameRate:  floatPtr(24.0),
					URI:        "stream2.m3u8",
				},
			},
			Renditions: []*MultivariantRendition{
				{
					Type:       MultivariantRenditionTypeAudio,
					URI:        "audio.m3u8",
					GroupID:    "aud1",
					Language:   "en",
					Name:       "english",
					Autoselect: true,
					Default:    true,
					Channels:   "2",
				},
				{
					Type:       MultivariantRenditionTypeSubtitles,
					URI:        "sub.m3u8",
					GroupID:    "sub1",
					Language:   "en",
					Name:       "english",
					Autoselect: true,
					Default:    true,
					Forced:     boolPtr(false),
				},
			},
		},
	},
	{
		"apple",
		`#EXTM3U
#EXT-X-VERSION:6
#EXT-X-INDEPENDENT-SEGMENTS

#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=2168183,BANDWIDTH=2177116,CODECS="avc1.640020,mp4a.40.2",RESOLUTION=960x540,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud1",SUBTITLES="sub1"
v5/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=7968416,BANDWIDTH=8001098,CODECS="avc1.64002a,mp4a.40.2",RESOLUTION=1920x1080,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud1",SUBTITLES="sub1"
v9/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=6170000,BANDWIDTH=6312875,CODECS="avc1.64002a,mp4a.40.2",RESOLUTION=1920x1080,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud1",SUBTITLES="sub1"
v8/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=4670769,BANDWIDTH=4943747,CODECS="avc1.64002a,mp4a.40.2",RESOLUTION=1920x1080,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud1",SUBTITLES="sub1"
v7/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=3168702,BANDWIDTH=3216424,CODECS="avc1.640020,mp4a.40.2",RESOLUTION=1280x720,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud1",SUBTITLES="sub1"
v6/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=1265132,BANDWIDTH=1268994,CODECS="avc1.64001e,mp4a.40.2",RESOLUTION=768x432,FRAME-RATE=30.000,CLOSED-CAPTIONS="cc1",AUDIO="aud1",SUBTITLES="sub1"
v4/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=895755,BANDWIDTH=902298,CODECS="avc1.64001e,mp4a.40.2",RESOLUTION=640x360,FRAME-RATE=30.000,CLOSED-CAPTIONS="cc1",AUDIO="aud1",SUBTITLES="sub1"
v3/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=530721,BANDWIDTH=541052,CODECS="avc1.640015,mp4a.40.2",RESOLUTION=480x270,FRAME-RATE=30.000,CLOSED-CAPTIONS="cc1",AUDIO="aud1",SUBTITLES="sub1"
v2/prog_index.m3u8

#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=2390686,BANDWIDTH=2399619,CODECS="avc1.640020,ac-3",RESOLUTION=960x540,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud2",SUBTITLES="sub1"
v5/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=8190919,BANDWIDTH=8223601,CODECS="avc1.64002a,ac-3",RESOLUTION=1920x1080,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud2",SUBTITLES="sub1"
v9/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=6392503,BANDWIDTH=6535378,CODECS="avc1.64002a,ac-3",RESOLUTION=1920x1080,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud2",SUBTITLES="sub1"
v8/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=4893272,BANDWIDTH=5166250,CODECS="avc1.64002a,ac-3",RESOLUTION=1920x1080,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud2",SUBTITLES="sub1"
v7/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=3391205,BANDWIDTH=3438927,CODECS="avc1.640020,ac-3",RESOLUTION=1280x720,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud2",SUBTITLES="sub1"
v6/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=1487635,BANDWIDTH=1491497,CODECS="avc1.64001e,ac-3",RESOLUTION=768x432,FRAME-RATE=30.000,CLOSED-CAPTIONS="cc1",AUDIO="aud2",SUBTITLES="sub1"
v4/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=1118258,BANDWIDTH=1124801,CODECS="avc1.64001e,ac-3",RESOLUTION=640x360,FRAME-RATE=30.000,CLOSED-CAPTIONS="cc1",AUDIO="aud2",SUBTITLES="sub1"
v3/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=753224,BANDWIDTH=763555,CODECS="avc1.640015,ac-3",RESOLUTION=480x270,FRAME-RATE=30.000,CLOSED-CAPTIONS="cc1",AUDIO="aud2",SUBTITLES="sub1"
v2/prog_index.m3u8

#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=2198686,BANDWIDTH=2207619,CODECS="avc1.640020,ec-3",RESOLUTION=960x540,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud3",SUBTITLES="sub1"
v5/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=7998919,BANDWIDTH=8031601,CODECS="avc1.64002a,ec-3",RESOLUTION=1920x1080,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud3",SUBTITLES="sub1"
v9/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=6200503,BANDWIDTH=6343378,CODECS="avc1.64002a,ec-3",RESOLUTION=1920x1080,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud3",SUBTITLES="sub1"
v8/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=4701272,BANDWIDTH=4974250,CODECS="avc1.64002a,ec-3",RESOLUTION=1920x1080,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud3",SUBTITLES="sub1"
v7/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=3199205,BANDWIDTH=3246927,CODECS="avc1.640020,ec-3",RESOLUTION=1280x720,FRAME-RATE=60.000,CLOSED-CAPTIONS="cc1",AUDIO="aud3",SUBTITLES="sub1"
v6/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=1295635,BANDWIDTH=1299497,CODECS="avc1.64001e,ec-3",RESOLUTION=768x432,FRAME-RATE=30.000,CLOSED-CAPTIONS="cc1",AUDIO="aud3",SUBTITLES="sub1"
v4/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=926258,BANDWIDTH=932801,CODECS="avc1.64001e,ec-3",RESOLUTION=640x360,FRAME-RATE=30.000,CLOSED-CAPTIONS="cc1",AUDIO="aud3",SUBTITLES="sub1"
v3/prog_index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=561224,BANDWIDTH=571555,CODECS="avc1.640015,ec-3",RESOLUTION=480x270,FRAME-RATE=30.000,CLOSED-CAPTIONS="cc1",AUDIO="aud3",SUBTITLES="sub1"
v2/prog_index.m3u8

#EXT-X-I-FRAME-STREAM-INF:AVERAGE-BANDWIDTH=183689,BANDWIDTH=187492,CODECS="avc1.64002a",RESOLUTION=1920x1080,URI="v7/iframe_index.m3u8"
#EXT-X-I-FRAME-STREAM-INF:AVERAGE-BANDWIDTH=132672,BANDWIDTH=136398,CODECS="avc1.640020",RESOLUTION=1280x720,URI="v6/iframe_index.m3u8"
#EXT-X-I-FRAME-STREAM-INF:AVERAGE-BANDWIDTH=97767,BANDWIDTH=101378,CODECS="avc1.640020",RESOLUTION=960x540,URI="v5/iframe_index.m3u8"
#EXT-X-I-FRAME-STREAM-INF:AVERAGE-BANDWIDTH=75722,BANDWIDTH=77818,CODECS="avc1.64001e",RESOLUTION=768x432,URI="v4/iframe_index.m3u8"
#EXT-X-I-FRAME-STREAM-INF:AVERAGE-BANDWIDTH=63522,BANDWIDTH=65091,CODECS="avc1.64001e",RESOLUTION=640x360,URI="v3/iframe_index.m3u8"
#EXT-X-I-FRAME-STREAM-INF:AVERAGE-BANDWIDTH=39678,BANDWIDTH=40282,CODECS="avc1.640015",RESOLUTION=480x270,URI="v2/iframe_index.m3u8"

#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud1",LANGUAGE="en",NAME="English",AUTOSELECT=YES,DEFAULT=YES,CHANNELS="2",URI="a1/prog_index.m3u8"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud2",LANGUAGE="en",NAME="English",AUTOSELECT=YES,DEFAULT=YES,CHANNELS="6",URI="a2/prog_index.m3u8"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud3",LANGUAGE="en",NAME="English",AUTOSELECT=YES,DEFAULT=YES,CHANNELS="6",URI="a3/prog_index.m3u8"

#EXT-X-MEDIA:TYPE=CLOSED-CAPTIONS,GROUP-ID="cc1",LANGUAGE="en",NAME="English",AUTOSELECT=YES,DEFAULT=YES,INSTREAM-ID="CC1"

#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="sub1",LANGUAGE="en",NAME="English",AUTOSELECT=YES,DEFAULT=YES,FORCED=NO,URI="s1/en/prog_index.m3u8"
`,
		`#EXTM3U
#EXT-X-VERSION:6
#EXT-X-INDEPENDENT-SEGMENTS

#EXT-X-STREAM-INF:BANDWIDTH=2177116,AVERAGE-BANDWIDTH=2168183,CODECS="avc1.640020,mp4a.40.2",RESOLUTION=960x540,FRAME-RATE=60.000,AUDIO="aud1",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v5/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=8001098,AVERAGE-BANDWIDTH=7968416,CODECS="avc1.64002a,mp4a.40.2",RESOLUTION=1920x1080,FRAME-RATE=60.000,AUDIO="aud1",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v9/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=6312875,AVERAGE-BANDWIDTH=6170000,CODECS="avc1.64002a,mp4a.40.2",RESOLUTION=1920x1080,FRAME-RATE=60.000,AUDIO="aud1",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v8/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=4943747,AVERAGE-BANDWIDTH=4670769,CODECS="avc1.64002a,mp4a.40.2",RESOLUTION=1920x1080,FRAME-RATE=60.000,AUDIO="aud1",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v7/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=3216424,AVERAGE-BANDWIDTH=3168702,CODECS="avc1.640020,mp4a.40.2",RESOLUTION=1280x720,FRAME-RATE=60.000,AUDIO="aud1",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v6/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1268994,AVERAGE-BANDWIDTH=1265132,CODECS="avc1.64001e,mp4a.40.2",RESOLUTION=768x432,FRAME-RATE=30.000,AUDIO="aud1",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v4/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=902298,AVERAGE-BANDWIDTH=895755,CODECS="avc1.64001e,mp4a.40.2",RESOLUTION=640x360,FRAME-RATE=30.000,AUDIO="aud1",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v3/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=541052,AVERAGE-BANDWIDTH=530721,CODECS="avc1.640015,mp4a.40.2",RESOLUTION=480x270,FRAME-RATE=30.000,AUDIO="aud1",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v2/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2399619,AVERAGE-BANDWIDTH=2390686,CODECS="avc1.640020,ac-3",RESOLUTION=960x540,FRAME-RATE=60.000,AUDIO="aud2",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v5/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=8223601,AVERAGE-BANDWIDTH=8190919,CODECS="avc1.64002a,ac-3",RESOLUTION=1920x1080,FRAME-RATE=60.000,AUDIO="aud2",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v9/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=6535378,AVERAGE-BANDWIDTH=6392503,CODECS="avc1.64002a,ac-3",RESOLUTION=1920x1080,FRAME-RATE=60.000,AUDIO="aud2",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v8/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=5166250,AVERAGE-BANDWIDTH=4893272,CODECS="avc1.64002a,ac-3",RESOLUTION=1920x1080,FRAME-RATE=60.000,AUDIO="aud2",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v7/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=3438927,AVERAGE-BANDWIDTH=3391205,CODECS="avc1.640020,ac-3",RESOLUTION=1280x720,FRAME-RATE=60.000,AUDIO="aud2",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v6/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1491497,AVERAGE-BANDWIDTH=1487635,CODECS="avc1.64001e,ac-3",RESOLUTION=768x432,FRAME-RATE=30.000,AUDIO="aud2",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v4/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1124801,AVERAGE-BANDWIDTH=1118258,CODECS="avc1.64001e,ac-3",RESOLUTION=640x360,FRAME-RATE=30.000,AUDIO="aud2",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v3/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=763555,AVERAGE-BANDWIDTH=753224,CODECS="avc1.640015,ac-3",RESOLUTION=480x270,FRAME-RATE=30.000,AUDIO="aud2",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v2/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2207619,AVERAGE-BANDWIDTH=2198686,CODECS="avc1.640020,ec-3",RESOLUTION=960x540,FRAME-RATE=60.000,AUDIO="aud3",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v5/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=8031601,AVERAGE-BANDWIDTH=7998919,CODECS="avc1.64002a,ec-3",RESOLUTION=1920x1080,FRAME-RATE=60.000,AUDIO="aud3",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v9/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=6343378,AVERAGE-BANDWIDTH=6200503,CODECS="avc1.64002a,ec-3",RESOLUTION=1920x1080,FRAME-RATE=60.000,AUDIO="aud3",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v8/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=4974250,AVERAGE-BANDWIDTH=4701272,CODECS="avc1.64002a,ec-3",RESOLUTION=1920x1080,FRAME-RATE=60.000,AUDIO="aud3",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v7/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=3246927,AVERAGE-BANDWIDTH=3199205,CODECS="avc1.640020,ec-3",RESOLUTION=1280x720,FRAME-RATE=60.000,AUDIO="aud3",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v6/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1299497,AVERAGE-BANDWIDTH=1295635,CODECS="avc1.64001e,ec-3",RESOLUTION=768x432,FRAME-RATE=30.000,AUDIO="aud3",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v4/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=932801,AVERAGE-BANDWIDTH=926258,CODECS="avc1.64001e,ec-3",RESOLUTION=640x360,FRAME-RATE=30.000,AUDIO="aud3",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v3/prog_index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=571555,AVERAGE-BANDWIDTH=561224,CODECS="avc1.640015,ec-3",RESOLUTION=480x270,FRAME-RATE=30.000,AUDIO="aud3",SUBTITLES="sub1",CLOSED-CAPTIONS="cc1"
v2/prog_index.m3u8

#EXT-X-MEDIA:TYPE="AUDIO",GROUP-ID="aud1",LANGUAGE="en",NAME="English",DEFAULT=YES,AUTOSELECT=YES,CHANNELS="2",URI="a1/prog_index.m3u8"
#EXT-X-MEDIA:TYPE="AUDIO",GROUP-ID="aud2",LANGUAGE="en",NAME="English",DEFAULT=YES,AUTOSELECT=YES,CHANNELS="6",URI="a2/prog_index.m3u8"
#EXT-X-MEDIA:TYPE="AUDIO",GROUP-ID="aud3",LANGUAGE="en",NAME="English",DEFAULT=YES,AUTOSELECT=YES,CHANNELS="6",URI="a3/prog_index.m3u8"
#EXT-X-MEDIA:TYPE="CLOSED-CAPTIONS",GROUP-ID="cc1",LANGUAGE="en",NAME="English",DEFAULT=YES,AUTOSELECT=YES,URI=""
#EXT-X-MEDIA:TYPE="SUBTITLES",GROUP-ID="sub1",LANGUAGE="en",NAME="English",DEFAULT=YES,AUTOSELECT=YES,FORCED=NO,URI="s1/en/prog_index.m3u8"
`,
		Multivariant{
			Version:             6,
			IndependentSegments: true,
			Variants: []*MultivariantVariant{
				{
					Bandwidth:        2177116,
					AverageBandwidth: intPtr(2168183),
					Codecs: []string{
						"avc1.640020",
						"mp4a.40.2",
					},
					Resolution:     "960x540",
					FrameRate:      floatPtr(60),
					Audio:          "aud1",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v5/prog_index.m3u8",
				},
				{
					Bandwidth:        8001098,
					AverageBandwidth: intPtr(7968416),
					Codecs: []string{
						"avc1.64002a",
						"mp4a.40.2",
					},
					Resolution:     "1920x1080",
					FrameRate:      floatPtr(60),
					Audio:          "aud1",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v9/prog_index.m3u8",
				},
				{
					Bandwidth:        6312875,
					AverageBandwidth: intPtr(6170000),
					Codecs: []string{
						"avc1.64002a",
						"mp4a.40.2",
					},
					Resolution:     "1920x1080",
					FrameRate:      floatPtr(60),
					Audio:          "aud1",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v8/prog_index.m3u8",
				},
				{
					Bandwidth:        4943747,
					AverageBandwidth: intPtr(4670769),
					Codecs: []string{
						"avc1.64002a",
						"mp4a.40.2",
					},
					Resolution:     "1920x1080",
					FrameRate:      floatPtr(60),
					Audio:          "aud1",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v7/prog_index.m3u8",
				},
				{
					Bandwidth:        3216424,
					AverageBandwidth: intPtr(3168702),
					Codecs: []string{
						"avc1.640020",
						"mp4a.40.2",
					},
					Resolution:     "1280x720",
					FrameRate:      floatPtr(60),
					Audio:          "aud1",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v6/prog_index.m3u8",
				},
				{
					Bandwidth:        1268994,
					AverageBandwidth: intPtr(1265132),
					Codecs: []string{
						"avc1.64001e",
						"mp4a.40.2",
					},
					Resolution:     "768x432",
					FrameRate:      floatPtr(30),
					Audio:          "aud1",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v4/prog_index.m3u8",
				},
				{
					Bandwidth:        902298,
					AverageBandwidth: intPtr(895755),
					Codecs: []string{
						"avc1.64001e",
						"mp4a.40.2",
					},
					Resolution:     "640x360",
					FrameRate:      floatPtr(30),
					Audio:          "aud1",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v3/prog_index.m3u8",
				},
				{
					Bandwidth:        541052,
					AverageBandwidth: intPtr(530721),
					Codecs: []string{
						"avc1.640015",
						"mp4a.40.2",
					},
					Resolution:     "480x270",
					FrameRate:      floatPtr(30),
					Audio:          "aud1",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v2/prog_index.m3u8",
				},
				{
					Bandwidth:        2399619,
					AverageBandwidth: intPtr(2390686),
					Codecs: []string{
						"avc1.640020",
						"ac-3",
					},
					Resolution:     "960x540",
					FrameRate:      floatPtr(60),
					Audio:          "aud2",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v5/prog_index.m3u8",
				},
				{
					Bandwidth:        8223601,
					AverageBandwidth: intPtr(8190919),
					Codecs: []string{
						"avc1.64002a",
						"ac-3",
					},
					Resolution:     "1920x1080",
					FrameRate:      floatPtr(60),
					Audio:          "aud2",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v9/prog_index.m3u8",
				},
				{
					Bandwidth:        6535378,
					AverageBandwidth: intPtr(6392503),
					Codecs: []string{
						"avc1.64002a",
						"ac-3",
					},
					Resolution:     "1920x1080",
					FrameRate:      floatPtr(60),
					Audio:          "aud2",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v8/prog_index.m3u8",
				},
				{
					Bandwidth:        5166250,
					AverageBandwidth: intPtr(4893272),
					Codecs: []string{
						"avc1.64002a",
						"ac-3",
					},
					Resolution:     "1920x1080",
					FrameRate:      floatPtr(60),
					Audio:          "aud2",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v7/prog_index.m3u8",
				},
				{
					Bandwidth:        3438927,
					AverageBandwidth: intPtr(3391205),
					Codecs: []string{
						"avc1.640020",
						"ac-3",
					},
					Resolution:     "1280x720",
					FrameRate:      floatPtr(60),
					Audio:          "aud2",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v6/prog_index.m3u8",
				},
				{
					Bandwidth:        1491497,
					AverageBandwidth: intPtr(1487635),
					Codecs: []string{
						"avc1.64001e",
						"ac-3",
					},
					Resolution:     "768x432",
					FrameRate:      floatPtr(30),
					Audio:          "aud2",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v4/prog_index.m3u8",
				},
				{
					Bandwidth:        1124801,
					AverageBandwidth: intPtr(1118258),
					Codecs: []string{
						"avc1.64001e",
						"ac-3",
					},
					Resolution:     "640x360",
					FrameRate:      floatPtr(30),
					Audio:          "aud2",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v3/prog_index.m3u8",
				},
				{
					Bandwidth:        763555,
					AverageBandwidth: intPtr(753224),
					Codecs: []string{
						"avc1.640015",
						"ac-3",
					},
					Resolution:     "480x270",
					FrameRate:      floatPtr(30),
					Audio:          "aud2",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v2/prog_index.m3u8",
				},
				{
					Bandwidth:        2207619,
					AverageBandwidth: intPtr(2198686),
					Codecs: []string{
						"avc1.640020",
						"ec-3",
					},
					Resolution:     "960x540",
					FrameRate:      floatPtr(60),
					Audio:          "aud3",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v5/prog_index.m3u8",
				},
				{
					Bandwidth:        8031601,
					AverageBandwidth: intPtr(7998919),
					Codecs: []string{
						"avc1.64002a",
						"ec-3",
					},
					Resolution:     "1920x1080",
					FrameRate:      floatPtr(60),
					Audio:          "aud3",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v9/prog_index.m3u8",
				},
				{
					Bandwidth:        6343378,
					AverageBandwidth: intPtr(6200503),
					Codecs: []string{
						"avc1.64002a",
						"ec-3",
					},
					Resolution:     "1920x1080",
					FrameRate:      floatPtr(60),
					Audio:          "aud3",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v8/prog_index.m3u8",
				},
				{
					Bandwidth:        4974250,
					AverageBandwidth: intPtr(4701272),
					Codecs: []string{
						"avc1.64002a",
						"ec-3",
					},
					Resolution:     "1920x1080",
					FrameRate:      floatPtr(60),
					Audio:          "aud3",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v7/prog_index.m3u8",
				},
				{
					Bandwidth:        3246927,
					AverageBandwidth: intPtr(3199205),
					Codecs: []string{
						"avc1.640020",
						"ec-3",
					},
					Resolution:     "1280x720",
					FrameRate:      floatPtr(60),
					Audio:          "aud3",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v6/prog_index.m3u8",
				},
				{
					Bandwidth:        1299497,
					AverageBandwidth: intPtr(1295635),
					Codecs: []string{
						"avc1.64001e",
						"ec-3",
					},
					Resolution:     "768x432",
					FrameRate:      floatPtr(30),
					Audio:          "aud3",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v4/prog_index.m3u8",
				},
				{
					Bandwidth:        932801,
					AverageBandwidth: intPtr(926258),
					Codecs: []string{
						"avc1.64001e",
						"ec-3",
					},
					Resolution:     "640x360",
					FrameRate:      floatPtr(30),
					Audio:          "aud3",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v3/prog_index.m3u8",
				},
				{
					Bandwidth:        571555,
					AverageBandwidth: intPtr(561224),
					Codecs: []string{
						"avc1.640015",
						"ec-3",
					},
					Resolution:     "480x270",
					FrameRate:      floatPtr(30),
					Audio:          "aud3",
					Subtitles:      "sub1",
					ClosedCaptions: "cc1",
					URI:            "v2/prog_index.m3u8",
				},
			},
			Renditions: []*MultivariantRendition{
				{
					Type:       MultivariantRenditionTypeAudio,
					URI:        "a1/prog_index.m3u8",
					GroupID:    "aud1",
					Language:   "en",
					Name:       "English",
					Default:    true,
					Autoselect: true,
					Channels:   "2",
				},
				{
					Type:       MultivariantRenditionTypeAudio,
					URI:        "a2/prog_index.m3u8",
					GroupID:    "aud2",
					Language:   "en",
					Name:       "English",
					Default:    true,
					Autoselect: true,
					Channels:   "6",
				},
				{
					Type:       MultivariantRenditionTypeAudio,
					URI:        "a3/prog_index.m3u8",
					GroupID:    "aud3",
					Language:   "en",
					Name:       "English",
					Default:    true,
					Autoselect: true,
					Channels:   "6",
				},
				{
					Type:       MultivariantRenditionTypeClosedCaptions,
					GroupID:    "cc1",
					Language:   "en",
					Name:       "English",
					Default:    true,
					Autoselect: true,
					InstreamID: "CC1",
				},
				{
					Type:       MultivariantRenditionTypeSubtitles,
					URI:        "s1/en/prog_index.m3u8",
					GroupID:    "sub1",
					Language:   "en",
					Name:       "English",
					Default:    true,
					Autoselect: true,
					Forced:     boolPtr(false),
				},
			},
		},
	},
	{
		"azure",
		`#EXTM3U
#EXT-X-VERSION:4
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="AAC_und_ch2_128kbps",URI="QualityLevels(125615)/Manifest(AAC_und_ch2_128kbps,format=m3u8-aapl)"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="AAC_und_ch2_56kbps",DEFAULT=YES,URI="QualityLevels(53620)/Manifest(AAC_und_ch2_56kbps,format=m3u8-aapl)"
#EXT-X-STREAM-INF:BANDWIDTH=546902,RESOLUTION=320x180,CODECS="avc1.64000d,mp4a.40.2",AUDIO="audio"
QualityLevels(393546)/Manifest(video,format=m3u8-aapl)
#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=546902,RESOLUTION=320x180,CODECS="avc1.64000d",URI="QualityLevels(393546)/Manifest(video,format=m3u8-aapl,type=keyframes)"
#EXT-X-STREAM-INF:BANDWIDTH=801672,RESOLUTION=640x360,CODECS="avc1.64001e,mp4a.40.2",AUDIO="audio"
QualityLevels(642832)/Manifest(video,format=m3u8-aapl)
#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=801672,RESOLUTION=640x360,CODECS="avc1.64001e",URI="QualityLevels(642832)/Manifest(video,format=m3u8-aapl,type=keyframes)"
#EXT-X-STREAM-INF:BANDWIDTH=1158387,RESOLUTION=640x360,CODECS="avc1.64001e,mp4a.40.2",AUDIO="audio"
QualityLevels(991868)/Manifest(video,format=m3u8-aapl)
#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=1158387,RESOLUTION=640x360,CODECS="avc1.64001e",URI="QualityLevels(991868)/Manifest(video,format=m3u8-aapl,type=keyframes)"
#EXT-X-STREAM-INF:BANDWIDTH=1667928,RESOLUTION=960x540,CODECS="avc1.64001f,mp4a.40.2",AUDIO="audio"
QualityLevels(1490441)/Manifest(video,format=m3u8-aapl)
#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=1667928,RESOLUTION=960x540,CODECS="avc1.64001f",URI="QualityLevels(1490441)/Manifest(video,format=m3u8-aapl,type=keyframes)"
#EXT-X-STREAM-INF:BANDWIDTH=2432306,RESOLUTION=960x540,CODECS="avc1.64001f,mp4a.40.2",AUDIO="audio"
QualityLevels(2238364)/Manifest(video,format=m3u8-aapl)
#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=2432306,RESOLUTION=960x540,CODECS="avc1.64001f",URI="QualityLevels(2238364)/Manifest(video,format=m3u8-aapl,type=keyframes)"
#EXT-X-STREAM-INF:BANDWIDTH=3604342,RESOLUTION=1280x720,CODECS="avc1.64001f,mp4a.40.2",AUDIO="audio"
QualityLevels(3385171)/Manifest(video,format=m3u8-aapl)
#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=3604342,RESOLUTION=1280x720,CODECS="avc1.64001f",URI="QualityLevels(3385171)/Manifest(video,format=m3u8-aapl,type=keyframes)"
#EXT-X-STREAM-INF:BANDWIDTH=4929129,RESOLUTION=1920x1080,CODECS="avc1.640028,mp4a.40.2",AUDIO="audio"
QualityLevels(4681440)/Manifest(video,format=m3u8-aapl)
#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=4929129,RESOLUTION=1920x1080,CODECS="avc1.640028",URI="QualityLevels(4681440)/Manifest(video,format=m3u8-aapl,type=keyframes)"
#EXT-X-STREAM-INF:BANDWIDTH=6254125,RESOLUTION=1920x1080,CODECS="avc1.640028,mp4a.40.2",AUDIO="audio"
QualityLevels(5977913)/Manifest(video,format=m3u8-aapl)
#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=6254125,RESOLUTION=1920x1080,CODECS="avc1.640028",URI="QualityLevels(5977913)/Manifest(video,format=m3u8-aapl,type=keyframes)"
`,
		`#EXTM3U
#EXT-X-VERSION:4

#EXT-X-STREAM-INF:BANDWIDTH=546902,CODECS="avc1.64000d,mp4a.40.2",RESOLUTION=320x180,AUDIO="audio"
QualityLevels(393546)/Manifest(video,format=m3u8-aapl)
#EXT-X-STREAM-INF:BANDWIDTH=801672,CODECS="avc1.64001e,mp4a.40.2",RESOLUTION=640x360,AUDIO="audio"
QualityLevels(642832)/Manifest(video,format=m3u8-aapl)
#EXT-X-STREAM-INF:BANDWIDTH=1158387,CODECS="avc1.64001e,mp4a.40.2",RESOLUTION=640x360,AUDIO="audio"
QualityLevels(991868)/Manifest(video,format=m3u8-aapl)
#EXT-X-STREAM-INF:BANDWIDTH=1667928,CODECS="avc1.64001f,mp4a.40.2",RESOLUTION=960x540,AUDIO="audio"
QualityLevels(1490441)/Manifest(video,format=m3u8-aapl)
#EXT-X-STREAM-INF:BANDWIDTH=2432306,CODECS="avc1.64001f,mp4a.40.2",RESOLUTION=960x540,AUDIO="audio"
QualityLevels(2238364)/Manifest(video,format=m3u8-aapl)
#EXT-X-STREAM-INF:BANDWIDTH=3604342,CODECS="avc1.64001f,mp4a.40.2",RESOLUTION=1280x720,AUDIO="audio"
QualityLevels(3385171)/Manifest(video,format=m3u8-aapl)
#EXT-X-STREAM-INF:BANDWIDTH=4929129,CODECS="avc1.640028,mp4a.40.2",RESOLUTION=1920x1080,AUDIO="audio"
QualityLevels(4681440)/Manifest(video,format=m3u8-aapl)
#EXT-X-STREAM-INF:BANDWIDTH=6254125,CODECS="avc1.640028,mp4a.40.2",RESOLUTION=1920x1080,AUDIO="audio"
QualityLevels(5977913)/Manifest(video,format=m3u8-aapl)

#EXT-X-MEDIA:TYPE="AUDIO",GROUP-ID="audio",NAME="AAC_und_ch2_128kbps",URI="QualityLevels(125615)/Manifest(AAC_und_ch2_128kbps,format=m3u8-aapl)"
#EXT-X-MEDIA:TYPE="AUDIO",GROUP-ID="audio",NAME="AAC_und_ch2_56kbps",DEFAULT=YES,URI="QualityLevels(53620)/Manifest(AAC_und_ch2_56kbps,format=m3u8-aapl)"
`,
		Multivariant{
			Version: 4,
			Variants: []*MultivariantVariant{
				{
					Bandwidth: 546902,
					Codecs: []string{
						"avc1.64000d",
						"mp4a.40.2",
					},
					URI:        "QualityLevels(393546)/Manifest(video,format=m3u8-aapl)",
					Resolution: "320x180",
					Audio:      "audio",
				},
				{
					Bandwidth: 801672,
					Codecs: []string{
						"avc1.64001e",
						"mp4a.40.2",
					},
					URI:        "QualityLevels(642832)/Manifest(video,format=m3u8-aapl)",
					Resolution: "640x360",
					Audio:      "audio",
				},
				{
					Bandwidth: 1158387,
					Codecs: []string{
						"avc1.64001e",
						"mp4a.40.2",
					},
					URI:        "QualityLevels(991868)/Manifest(video,format=m3u8-aapl)",
					Resolution: "640x360",
					Audio:      "audio",
				},
				{
					Bandwidth: 1667928,
					Codecs: []string{
						"avc1.64001f",
						"mp4a.40.2",
					},
					URI:        "QualityLevels(1490441)/Manifest(video,format=m3u8-aapl)",
					Resolution: "960x540",
					Audio:      "audio",
				},
				{
					Bandwidth: 2432306,
					Codecs: []string{
						"avc1.64001f",
						"mp4a.40.2",
					},
					URI:        "QualityLevels(2238364)/Manifest(video,format=m3u8-aapl)",
					Resolution: "960x540",
					Audio:      "audio",
				},
				{
					Bandwidth: 3604342,
					Codecs: []string{
						"avc1.64001f",
						"mp4a.40.2",
					},
					URI:        "QualityLevels(3385171)/Manifest(video,format=m3u8-aapl)",
					Resolution: "1280x720",
					Audio:      "audio",
				},
				{
					Bandwidth: 4929129,
					Codecs: []string{
						"avc1.640028",
						"mp4a.40.2",
					},
					URI:        "QualityLevels(4681440)/Manifest(video,format=m3u8-aapl)",
					Resolution: "1920x1080",
					Audio:      "audio",
				},
				{
					Bandwidth: 6254125,
					Codecs: []string{
						"avc1.640028",
						"mp4a.40.2",
					},
					URI:        "QualityLevels(5977913)/Manifest(video,format=m3u8-aapl)",
					Resolution: "1920x1080",
					Audio:      "audio",
				},
			},
			Renditions: []*MultivariantRendition{
				{
					Type:    MultivariantRenditionTypeAudio,
					GroupID: "audio",
					URI:     "QualityLevels(125615)/Manifest(AAC_und_ch2_128kbps,format=m3u8-aapl)",
					Name:    "AAC_und_ch2_128kbps",
				},
				{
					Type:    MultivariantRenditionTypeAudio,
					GroupID: "audio",
					URI:     "QualityLevels(53620)/Manifest(AAC_und_ch2_56kbps,format=m3u8-aapl)",
					Name:    "AAC_und_ch2_56kbps",
					Default: true,
				},
			},
		},
	},
}

func TestMultivariantUnmarshal(t *testing.T) {
	for _, ca := range casesMultivariant {
		t.Run(ca.name, func(t *testing.T) {
			var m Multivariant
			err := m.Unmarshal([]byte(ca.input))
			require.NoError(t, err)
			require.Equal(t, ca.dec, m)
		})
	}
}

func TestMultivariantMarshal(t *testing.T) {
	for _, ca := range casesMultivariant {
		t.Run(ca.name, func(t *testing.T) {
			byts, err := ca.dec.Marshal()
			require.NoError(t, err)
			require.Equal(t, string(byts), ca.output)
		})
	}
}
