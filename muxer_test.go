package gohlslib

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

var testTime = time.Date(2010, 0o1, 0o1, 0o1, 0o1, 0o1, 0, time.UTC)

// baseline profile without POC
var testSPS = []byte{
	0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
	0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
	0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
	0x20,
}

var testPPS = []byte{0x01, 0x02, 0x03, 0x04}

var testConfig = mpeg4audio.Config{
	Type:         2,
	SampleRate:   44100,
	ChannelCount: 2,
}

var testVideoTrack = &Track{
	Codec: &codecs.H264{
		SPS: testSPS,
		PPS: []byte{0x08},
	},
	ClockRate: 90000,
}

var testAudioTrack = &Track{
	Codec: &codecs.MPEG4Audio{
		Config: mpeg4audio.Config{
			Type:         2,
			SampleRate:   44100,
			ChannelCount: 2,
		},
	},
	ClockRate: 44100,
}

var testAudioTrack2 = &Track{
	Codec: &codecs.MPEG4Audio{
		Config: mpeg4audio.Config{
			Type:         2,
			SampleRate:   44100,
			ChannelCount: 2,
		},
	},
	ClockRate: 44100,
	Name:      "German",
	Language:  "de",
}

type dummyResponseWriter struct {
	bytes.Buffer
	h          http.Header
	statusCode int
}

func (w *dummyResponseWriter) Header() http.Header {
	return w.h
}

func (w *dummyResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func doRequest(m *Muxer, pathAndQuery string) ([]byte, http.Header, error) {
	u, _ := url.Parse("http://localhost/" + pathAndQuery)

	r := &http.Request{
		URL: u,
	}

	w := &dummyResponseWriter{
		h: make(http.Header),
	}

	m.Handle(w, r)

	if w.statusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("bad status code: %v", w.statusCode)
	}

	return w.Bytes(), w.h, nil
}

func TestMuxer(t *testing.T) {
	createMuxer := func(t *testing.T, variant string, content string) *Muxer {
		var v MuxerVariant
		var segmentCount int

		switch variant {
		case "mpegts":
			v = MuxerVariantMPEGTS
			segmentCount = 3

		case "fmp4":
			v = MuxerVariantFMP4
			segmentCount = 3

		case "lowLatency":
			v = MuxerVariantLowLatency
			segmentCount = 7
		}

		var tracks []*Track

		switch content {
		case "video+audio":
			tracks = append(tracks, testVideoTrack)
			tracks = append(tracks, testAudioTrack)

		case "video":
			tracks = append(tracks, testVideoTrack)

		case "audio":
			tracks = append(tracks, testAudioTrack)

		case "video+multiaudio":
			tracks = append(tracks, testVideoTrack)
			tracks = append(tracks, testAudioTrack)
			tracks = append(tracks, testAudioTrack2)

		case "multiaudio":
			tracks = append(tracks, testAudioTrack)
			tracks = append(tracks, testAudioTrack2)
		}

		m := &Muxer{
			Variant:            v,
			SegmentCount:       segmentCount,
			SegmentMinDuration: 1 * time.Second,
			Tracks:             tracks,
		}

		err := m.Start()
		require.NoError(t, err)

		switch content {
		case "video+audio":
			d := 1 * time.Second
			err = m.WriteH264(testVideoTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testVideoTrack.ClockRate)/int64(time.Second),
				[][]byte{
					{1}, // non-IDR
				})
			require.NoError(t, err)

			d = 2 * time.Second
			err = m.WriteH264(testVideoTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testVideoTrack.ClockRate)/int64(time.Second),
				[][]byte{
					testSPS, // SPS
					{8},     // PPS
					{5},     // IDR
				})
			require.NoError(t, err)

			d = 3 * time.Second
			err = m.WriteMPEG4Audio(testAudioTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testAudioTrack.ClockRate)/int64(time.Second),
				[][]byte{{
					0x01, 0x02, 0x03, 0x04,
				}})
			require.NoError(t, err)

			d = 3500 * time.Millisecond
			err = m.WriteMPEG4Audio(testAudioTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testAudioTrack.ClockRate)/int64(time.Second),
				[][]byte{{
					0x01, 0x02, 0x03, 0x04,
				}})
			require.NoError(t, err)

			d = 4 * time.Second
			err = m.WriteH264(testVideoTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testVideoTrack.ClockRate)/int64(time.Second),
				[][]byte{
					{1}, // non-IDR
				})
			require.NoError(t, err)

			d = 4500 * time.Millisecond
			err = m.WriteMPEG4Audio(testAudioTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testAudioTrack.ClockRate)/int64(time.Second),
				[][]byte{{
					0x01, 0x02, 0x03, 0x04,
				}})
			require.NoError(t, err)

			d = 6 * time.Second
			err = m.WriteH264(testVideoTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testVideoTrack.ClockRate)/int64(time.Second),
				[][]byte{
					{5}, // IDR
				})
			require.NoError(t, err)

			d = 7 * time.Second
			err = m.WriteH264(testVideoTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testVideoTrack.ClockRate)/int64(time.Second),
				[][]byte{
					{5}, // IDR
				})
			require.NoError(t, err)

		case "video":
			d := 2 * time.Second
			err = m.WriteH264(testVideoTrack, testTime.Add(d-2*time.Second),
				int64(d)*int64(testVideoTrack.ClockRate)/int64(time.Second),
				[][]byte{
					testSPS, // SPS
					{8},     // PPS
					{5},     // IDR
				})
			require.NoError(t, err)

			d = 6 * time.Second
			err = m.WriteH264(testVideoTrack, testTime.Add(d-2*time.Second),
				int64(d)*int64(testVideoTrack.ClockRate)/int64(time.Second),
				[][]byte{
					{5}, // IDR
				})
			require.NoError(t, err)

			d = 7 * time.Second
			err = m.WriteH264(testVideoTrack, testTime.Add(d-2*time.Second),
				int64(d)*int64(testVideoTrack.ClockRate)/int64(time.Second),
				[][]byte{
					{5}, // IDR
				})
			require.NoError(t, err)

		case "audio":
			for i := 0; i < 100; i++ {
				d := time.Duration(i) * 4 * time.Millisecond
				err = m.WriteMPEG4Audio(testAudioTrack, testTime.Add(d-1*time.Second),
					int64(d)*int64(testAudioTrack.ClockRate)/int64(time.Second),
					[][]byte{{
						0x01, 0x02, 0x03, 0x04,
					}})
				require.NoError(t, err)
			}

			d := 2 * time.Second
			err = m.WriteMPEG4Audio(testAudioTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testAudioTrack.ClockRate)/int64(time.Second),
				[][]byte{{
					0x01, 0x02, 0x03, 0x04,
				}})
			require.NoError(t, err)

			d = 3 * time.Second
			err = m.WriteMPEG4Audio(testAudioTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testAudioTrack.ClockRate)/int64(time.Second),
				[][]byte{{
					0x01, 0x02, 0x03, 0x04,
				}})
			require.NoError(t, err)

		case "video+multiaudio":
			d := 2 * time.Second
			err = m.WriteH264(testVideoTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testVideoTrack.ClockRate)/int64(time.Second),
				[][]byte{
					testSPS, // SPS
					{8},     // PPS
					{5},     // IDR
				})
			require.NoError(t, err)

			d = 3 * time.Second
			err = m.WriteMPEG4Audio(testAudioTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testAudioTrack.ClockRate)/int64(time.Second),
				[][]byte{{
					0x01, 0x02, 0x03, 0x04,
				}})
			require.NoError(t, err)

			d = 3500 * time.Millisecond
			err = m.WriteMPEG4Audio(testAudioTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testAudioTrack.ClockRate)/int64(time.Second),
				[][]byte{{
					0x01, 0x02, 0x03, 0x04,
				}})
			require.NoError(t, err)

			d = 2 * time.Second
			err = m.WriteMPEG4Audio(testAudioTrack2, testTime.Add(d-1*time.Second),
				int64(d)*int64(testAudioTrack.ClockRate)/int64(time.Second),
				[][]byte{{
					0x01, 0x02, 0x03, 0x04,
				}})
			require.NoError(t, err)

			d = 2500 * time.Millisecond
			err = m.WriteMPEG4Audio(testAudioTrack2, testTime.Add(d-1*time.Second),
				int64(d)*int64(testAudioTrack.ClockRate)/int64(time.Second),
				[][]byte{{
					0x01, 0x02, 0x03, 0x04,
				}})
			require.NoError(t, err)

			d = 6 * time.Second
			err = m.WriteH264(testVideoTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testVideoTrack.ClockRate)/int64(time.Second),
				[][]byte{
					{5}, // IDR
				})
			require.NoError(t, err)

			d = 7 * time.Second
			err = m.WriteH264(testVideoTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testVideoTrack.ClockRate)/int64(time.Second),
				[][]byte{
					{5}, // IDR
				})
			require.NoError(t, err)

		case "multiaudio":
			for i := 0; i < 100; i++ {
				d := time.Duration(i) * 4 * time.Millisecond
				err = m.WriteMPEG4Audio(testAudioTrack, testTime.Add(d-1*time.Second),
					int64(d)*int64(testAudioTrack.ClockRate)/int64(time.Second),
					[][]byte{{
						0x01, 0x02, 0x03, 0x04,
					}})
				require.NoError(t, err)

				err = m.WriteMPEG4Audio(testAudioTrack2, testTime.Add(d-1*time.Second),
					int64(d)*int64(testAudioTrack.ClockRate)/int64(time.Second),
					[][]byte{{
						0x01, 0x02, 0x03, 0x04,
					}})
				require.NoError(t, err)
			}

			d := 2 * time.Second
			err = m.WriteMPEG4Audio(testAudioTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testAudioTrack.ClockRate)/int64(time.Second),
				[][]byte{{
					0x01, 0x02, 0x03, 0x04,
				}})
			require.NoError(t, err)

			d = 3 * time.Second
			err = m.WriteMPEG4Audio(testAudioTrack, testTime.Add(d-1*time.Second),
				int64(d)*int64(testAudioTrack.ClockRate)/int64(time.Second),
				[][]byte{{
					0x01, 0x02, 0x03, 0x04,
				}})
			require.NoError(t, err)
		}

		return m
	}

	checkMultivariantPlaylist := func(t *testing.T, m *Muxer, variant string, content string) {
		byts, h, err := doRequest(m, "/index.m3u8?key=value")
		require.NoError(t, err)
		require.Equal(t, "application/vnd.apple.mpegurl", h.Get("Content-Type"))
		require.Equal(t, "max-age=30", h.Get("Cache-Control"))

		switch {
		case content == "video+audio" && variant == "mpegts":
			require.Equal(t, "#EXTM3U\n"+
				"#EXT-X-VERSION:3\n"+
				"#EXT-X-INDEPENDENT-SEGMENTS\n"+
				"\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=4512,AVERAGE-BANDWIDTH=3008,"+
				"CODECS=\"avc1.42c028,mp4a.40.2\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
				"main_stream.m3u8?key=value\n", string(byts))

		case content == "video+audio" && variant == "fmp4":
			require.Equal(t, "#EXTM3U\n"+
				"#EXT-X-VERSION:9\n"+
				"#EXT-X-INDEPENDENT-SEGMENTS\n"+
				"\n"+
				"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio\","+
				"NAME=\"audio2\",AUTOSELECT=YES,DEFAULT=YES,URI=\"audio2_stream.m3u8?key=value\"\n"+
				"\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=872,AVERAGE-BANDWIDTH=436,CODECS=\"avc1.42c028,mp4a.40.2\","+
				"RESOLUTION=1920x1080,FRAME-RATE=30.000,AUDIO=\"audio\"\n"+
				"video1_stream.m3u8?key=value\n", string(byts))

		case content == "video+audio" && variant == "lowLatency":
			require.Equal(t, "#EXTM3U\n"+
				"#EXT-X-VERSION:9\n"+
				"#EXT-X-INDEPENDENT-SEGMENTS\n"+
				"\n"+
				"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio\","+
				"NAME=\"audio2\",AUTOSELECT=YES,DEFAULT=YES,URI=\"audio2_stream.m3u8?key=value\"\n"+
				"\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=872,AVERAGE-BANDWIDTH=584,CODECS=\"avc1.42c028,mp4a.40.2\","+
				"RESOLUTION=1920x1080,FRAME-RATE=30.000,AUDIO=\"audio\"\n"+
				"video1_stream.m3u8?key=value\n", string(byts))

		case content == "video" && variant == "mpegts":
			require.Equal(t, "#EXTM3U\n"+
				"#EXT-X-VERSION:3\n"+
				"#EXT-X-INDEPENDENT-SEGMENTS\n"+
				"\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=4512,AVERAGE-BANDWIDTH=1804,"+
				"CODECS=\"avc1.42c028\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
				"main_stream.m3u8?key=value\n", string(byts))

		case content == "video" && variant == "fmp4":
			require.Equal(t, "#EXTM3U\n"+
				"#EXT-X-VERSION:9\n"+
				"#EXT-X-INDEPENDENT-SEGMENTS\n"+
				"\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=872,AVERAGE-BANDWIDTH=403,CODECS=\"avc1.42c028\","+
				"RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
				"video1_stream.m3u8?key=value\n", string(byts))

		case content == "video" && variant == "lowLatency":
			require.Equal(t, "#EXTM3U\n"+
				"#EXT-X-VERSION:9\n"+
				"#EXT-X-INDEPENDENT-SEGMENTS\n"+
				"\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=872,AVERAGE-BANDWIDTH=403,CODECS=\"avc1.42c028\","+
				"RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
				"video1_stream.m3u8?key=value\n", string(byts))

		case content == "audio" && variant == "mpegts":
			require.Equal(t, "#EXTM3U\n"+
				"#EXT-X-VERSION:3\n"+
				"#EXT-X-INDEPENDENT-SEGMENTS\n"+
				"\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=225600,AVERAGE-BANDWIDTH=225600,CODECS=\"mp4a.40.2\"\n"+
				"main_stream.m3u8?key=value\n", string(byts))

		case content == "audio" && variant == "fmp4":
			require.Equal(t, "#EXTM3U\n"+
				"#EXT-X-VERSION:9\n"+
				"#EXT-X-INDEPENDENT-SEGMENTS\n"+
				"\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=5184,AVERAGE-BANDWIDTH=3744,CODECS=\"mp4a.40.2\"\n"+
				"audio1_stream.m3u8?key=value\n", string(byts))

		case content == "audio" && variant == "lowLatency":
			require.Equal(t, "#EXTM3U\n"+
				"#EXT-X-VERSION:9\n"+
				"#EXT-X-INDEPENDENT-SEGMENTS\n"+
				"\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=5568,AVERAGE-BANDWIDTH=4000,CODECS=\"mp4a.40.2\"\n"+
				"audio1_stream.m3u8?key=value\n", string(byts))

		case content == "video+multiaudio" && (variant == "fmp4" || variant == "lowLatency"):
			require.Equal(t, "#EXTM3U\n"+
				"#EXT-X-VERSION:9\n"+
				"#EXT-X-INDEPENDENT-SEGMENTS\n"+
				"\n"+
				"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio\","+
				"NAME=\"audio2\",AUTOSELECT=YES,DEFAULT=YES,URI=\"audio2_stream.m3u8?key=value\"\n"+
				"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio\","+
				"LANGUAGE=\"de\",NAME=\"German\",AUTOSELECT=YES,URI=\"audio3_stream.m3u8?key=value\"\n"+
				"\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=872,AVERAGE-BANDWIDTH=403,"+
				"CODECS=\"avc1.42c028,mp4a.40.2\",RESOLUTION=1920x1080,FRAME-RATE=30.000,AUDIO=\"audio\"\n"+
				"video1_stream.m3u8?key=value\n", string(byts))

		case content == "multiaudio" && variant == "fmp4":
			require.Equal(t, "#EXTM3U\n"+
				"#EXT-X-VERSION:9\n"+
				"#EXT-X-INDEPENDENT-SEGMENTS\n"+
				"\n"+
				"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio\",NAME=\"audio1\",AUTOSELECT=YES,DEFAULT=YES\n"+
				"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio\",LANGUAGE=\"de\",NAME=\"German\","+
				"AUTOSELECT=YES,URI=\"audio2_stream.m3u8?key=value\"\n"+
				"\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=5184,AVERAGE-BANDWIDTH=3744,CODECS=\"mp4a.40.2\",AUDIO=\"audio\"\n"+
				"audio1_stream.m3u8?key=value\n", string(byts))

		case content == "multiaudio" && variant == "lowLatency":
			require.Equal(t, "#EXTM3U\n"+
				"#EXT-X-VERSION:9\n"+
				"#EXT-X-INDEPENDENT-SEGMENTS\n"+
				"\n"+
				"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio\",NAME=\"audio1\",AUTOSELECT=YES,DEFAULT=YES\n"+
				"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio\",LANGUAGE=\"de\",NAME=\"German\","+
				"AUTOSELECT=YES,URI=\"audio2_stream.m3u8?key=value\"\n"+
				"\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=5568,AVERAGE-BANDWIDTH=4000,CODECS=\"mp4a.40.2\",AUDIO=\"audio\"\n"+
				"audio1_stream.m3u8?key=value\n", string(byts))
		}
	}

	checkPlaylist1 := func(t *testing.T, m *Muxer, variant string, content string) {
		var u string

		switch {
		case variant == "mpegts":
			u = "main_stream.m3u8?key=value"

		case content == "audio" || content == "multiaudio":
			u = "audio1_stream.m3u8?key=value"

		default:
			u = "video1_stream.m3u8?key=value"
		}

		byts, h, err := doRequest(m, u)
		require.NoError(t, err)
		require.Equal(t, "application/vnd.apple.mpegurl", h.Get("Content-Type"))
		require.Equal(t, "no-cache", h.Get("Cache-Control"))

		switch {
		case variant == "mpegts" && (content == "video+audio" || content == "video"):
			re := regexp.MustCompile(`^#EXTM3U\n` +
				`#EXT-X-VERSION:3\n` +
				`#EXT-X-ALLOW-CACHE:NO\n` +
				`#EXT-X-TARGETDURATION:4\n` +
				`#EXT-X-MEDIA-SEQUENCE:0\n` +
				`#EXT-X-PROGRAM-DATE-TIME:.*?\n` +
				`#EXTINF:4.00000,\n` +
				`(.*?_seg0\.ts\?key=value)\n` +
				`#EXT-X-PROGRAM-DATE-TIME:.*?\n` +
				`#EXTINF:1.00000,\n` +
				`(.*?_seg1\.ts\?key=value)\n$`)
			require.Regexp(t, re, string(byts))

		case variant == "mpegts" && content == "audio":
			re := regexp.MustCompile(`^#EXTM3U\n` +
				`#EXT-X-VERSION:3\n` +
				`#EXT-X-ALLOW-CACHE:NO\n` +
				`#EXT-X-TARGETDURATION:2\n` +
				`#EXT-X-MEDIA-SEQUENCE:0\n` +
				`#EXT-X-PROGRAM-DATE-TIME:.*?\n` +
				`#EXTINF:2.00000,\n` +
				`(.*?_seg0\.ts\?key=value)\n$`)
			require.Regexp(t, re, string(byts))

		case variant == "fmp4" && (content == "video+audio" || content == "video"):
			re := regexp.MustCompile(`^#EXTM3U\n` +
				`#EXT-X-VERSION:10\n` +
				`#EXT-X-TARGETDURATION:4\n` +
				`#EXT-X-MEDIA-SEQUENCE:0\n` +
				`#EXT-X-MAP:URI="(.*?_init.mp4\?key=value)"\n` +
				`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
				`#EXTINF:4.00000,\n` +
				`(.*?_seg0\.mp4\?key=value)\n` +
				`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
				`#EXTINF:1.00000,\n` +
				`(.*?_seg1\.mp4\?key=value)\n$`)
			require.Regexp(t, re, string(byts))

		case variant == "fmp4" && content == "audio":
			re := regexp.MustCompile(`^#EXTM3U\n` +
				`#EXT-X-VERSION:10\n` +
				`#EXT-X-TARGETDURATION:2\n` +
				`#EXT-X-MEDIA-SEQUENCE:0\n` +
				`#EXT-X-MAP:URI="(.*?_init.mp4\?key=value)"\n` +
				`#EXT-X-PROGRAM-DATE-TIME:.*?\n` +
				`#EXTINF:2.00000,\n` +
				`(.*?_seg0\.mp4\?key=value)\n` +
				`#EXT-X-PROGRAM-DATE-TIME:.*?\n` +
				`#EXTINF:1.00000,\n` +
				`(.*?_seg1.mp4\?key=value)\n$`)
			require.Regexp(t, re, string(byts))

		case variant == "lowLatency" && content == "video+audio":
			re := regexp.MustCompile(`^#EXTM3U\n` +
				`#EXT-X-VERSION:10\n` +
				`#EXT-X-TARGETDURATION:4\n` +
				`#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=5\.00000,CAN-SKIP-UNTIL=24\.00000\n` +
				`#EXT-X-PART-INF:PART-TARGET=2\.00000\n` +
				`#EXT-X-MEDIA-SEQUENCE:2\n` +
				`#EXT-X-MAP:URI="(.*?_init\.mp4\?key=value)"\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-PROGRAM-DATE-TIME:2010-01-01T01:01:02Z\n` +
				`#EXT-X-PART:DURATION=2.00000,URI="(.*?_part0\.mp4\?key=value)",INDEPENDENT=YES\n` +
				`#EXT-X-PART:DURATION=2.00000,URI="(.*?_part1\.mp4\?key=value)"\n` +
				`#EXTINF:4.00000,\n` +
				`(.*?_seg7\.mp4\?key=value)\n` +
				`#EXT-X-PROGRAM-DATE-TIME:2010-01-01T01:01:06Z\n` +
				`#EXT-X-PART:DURATION=1.00000,URI="(.*?_part2\.mp4\?key=value)",INDEPENDENT=YES\n` +
				`#EXTINF:1.00000,\n` +
				`(.*?_seg8\.mp4\?key=value)\n` +
				`#EXT-X-PRELOAD-HINT:TYPE=PART,URI="(.*?_part3\.mp4\?key=value)"\n$`)
			require.Regexp(t, re, string(byts))

		case variant == "lowLatency" && content == "video":
			re := regexp.MustCompile(`^#EXTM3U\n` +
				`#EXT-X-VERSION:10\n` +
				`#EXT-X-TARGETDURATION:4\n` +
				`#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=10\.00000,CAN-SKIP-UNTIL=24\.00000\n` +
				`#EXT-X-PART-INF:PART-TARGET=4\.00000\n` +
				`#EXT-X-MEDIA-SEQUENCE:2\n` +
				`#EXT-X-MAP:URI="(.*?_init\.mp4\?key=value)"\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-PROGRAM-DATE-TIME:.+?\n` +
				`#EXT-X-PART:DURATION=4.00000,URI="(.*?_part0\.mp4\?key=value)",INDEPENDENT=YES\n` +
				`#EXTINF:4.00000,\n` +
				`(.*?_seg7\.mp4\?key=value)\n` +
				`#EXT-X-PROGRAM-DATE-TIME:.+?\n` +
				`#EXT-X-PART:DURATION=1.00000,URI="(.*?_part1\.mp4\?key=value)",INDEPENDENT=YES\n` +
				`#EXTINF:1.00000,\n` +
				`(.*?_seg8\.mp4\?key=value)\n` +
				`#EXT-X-PRELOAD-HINT:TYPE=PART,URI="(.*?_part2\.mp4\?key=value)"\n$`)
			require.Regexp(t, re, string(byts))

		case variant == "lowLatency" && content == "audio":
			re := regexp.MustCompile(`^#EXTM3U\n` +
				`#EXT-X-VERSION:10\n` +
				`#EXT-X-TARGETDURATION:2\n` +
				`#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=4\.50000,CAN-SKIP-UNTIL=12\.00000\n` +
				`#EXT-X-PART-INF:PART-TARGET=1\.80000\n` +
				`#EXT-X-MEDIA-SEQUENCE:2\n` +
				`#EXT-X-MAP:URI="(.*?_init\.mp4\?key=value)"\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:2.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:2.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:2.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:2.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:2.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-PROGRAM-DATE-TIME:.+?\n` +
				`#EXT-X-PART:DURATION=0\.20000,URI="(.*?_part0\.mp4\?key=value)",INDEPENDENT=YES\n` +
				`#EXT-X-PART:DURATION=1\.80000,URI="(.*?_part1\.mp4\?key=value)",INDEPENDENT=YES\n` +
				`#EXTINF:2.00000,\n` +
				`(.*?_seg7\.mp4\?key=value)\n` +
				`#EXT-X-PROGRAM-DATE-TIME:.+?\n` +
				`#EXT-X-PART:DURATION=1.00000,URI="(.*?_part2\.mp4\?key=value)",INDEPENDENT=YES\n` +
				`#EXTINF:1.00000,\n` +
				`(.*?_seg8\.mp4\?key=value)\n` +
				`#EXT-X-PRELOAD-HINT:TYPE=PART,URI="(.*?_part3\.mp4\?key=value)"\n$`)
			require.Regexp(t, re, string(byts))
		}
	}

	checkPlaylist2 := func(t *testing.T, m *Muxer, variant string) {
		byts, h, err := doRequest(m, "audio2_stream.m3u8?key=value")
		require.NoError(t, err)
		require.Equal(t, "application/vnd.apple.mpegurl", h.Get("Content-Type"))
		require.Equal(t, "no-cache", h.Get("Cache-Control"))

		switch {
		case variant == "fmp4":
			re := regexp.MustCompile(`^#EXTM3U\n` +
				`#EXT-X-VERSION:10\n` +
				`#EXT-X-TARGETDURATION:4\n` +
				`#EXT-X-MEDIA-SEQUENCE:0\n` +
				`#EXT-X-MAP:URI="(.*?_init.mp4\?key=value)"\n` +
				`#EXT-X-PROGRAM-DATE-TIME:.*?\n` +
				`#EXTINF:4.00000,\n` +
				`(.*?_seg0\.mp4\?key=value)\n` +
				`#EXT-X-PROGRAM-DATE-TIME:.*?\n` +
				`#EXTINF:1.00000,\n` +
				`(.*?_seg1.mp4\?key=value)\n$`)
			require.Regexp(t, re, string(byts))

		case variant == "lowLatency":
			re := regexp.MustCompile(`^#EXTM3U\n` +
				`#EXT-X-VERSION:10\n` +
				`#EXT-X-TARGETDURATION:4\n` +
				`#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=5\.00000,CAN-SKIP-UNTIL=24\.00000\n` +
				`#EXT-X-PART-INF:PART-TARGET=2\.00000\n` +
				`#EXT-X-MEDIA-SEQUENCE:2\n` +
				`#EXT-X-MAP:URI="(.*?_init\.mp4\?key=value)"\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-GAP\n` +
				`#EXTINF:4.00000,\n` +
				`gap.mp4\n` +
				`#EXT-X-PROGRAM-DATE-TIME:.+?\n` +
				`#EXT-X-PART:DURATION=2\.00000,URI="(.*?_part0\.mp4\?key=value)"\n` +
				`#EXT-X-PART:DURATION=2\.00000,URI="(.*?_part1\.mp4\?key=value)",INDEPENDENT=YES\n` +
				`#EXTINF:4.00000,\n` +
				`(.*?_seg7\.mp4\?key=value)\n` +
				`#EXT-X-PROGRAM-DATE-TIME:.+?\n` +
				`#EXT-X-PART:DURATION=1.00000,URI="(.*?_part2\.mp4\?key=value)"\n` +
				`#EXTINF:1.00000,\n` +
				`(.*?_seg8\.mp4\?key=value)\n` +
				`#EXT-X-PRELOAD-HINT:TYPE=PART,URI="(.*?_part3\.mp4\?key=value)"\n$`)
			require.Regexp(t, re, string(byts))
		}
	}

	for _, variant := range []string{
		"mpegts",
		"fmp4",
		"lowLatency",
	} {
		for _, content := range []string{
			"video+audio",
			"video",
			"audio",
			"video+multiaudio",
			"multiaudio",
		} {
			if variant == "mpegts" && (content == "video+multiaudio" || content == "multiaudio") {
				continue
			}

			t.Run(variant+"_"+content, func(t *testing.T) {
				m := createMuxer(t, variant, content)
				defer m.Close()

				checkMultivariantPlaylist(t, m, variant, content)
				checkPlaylist1(t, m, variant, content)

				if (variant == "fmp4" || variant == "lowLatency") && content == "video+audio" {
					checkPlaylist2(t, m, variant)
				}
			})
		}
	}
}

func TestMuxerCloseBeforeData(t *testing.T) {
	m := &Muxer{
		Variant:            MuxerVariantFMP4,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack},
	}

	err := m.Start()
	require.NoError(t, err)

	m.Close()

	b, _, _ := doRequest(m, "index.m3u8")
	require.Equal(t, []byte(nil), b)

	b, _, _ = doRequest(m, "video1_stream.m3u8")
	require.Equal(t, []byte(nil), b)

	b, _, _ = doRequest(m, m.prefix+"_init.mp4")
	require.Equal(t, []byte(nil), b)
}

func TestMuxerMaxSegmentSize(t *testing.T) {
	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		SegmentMaxSize:     1,
		Tracks:             []*Track{testVideoTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(
		testVideoTrack,
		testTime,
		int64(2*time.Second)*int64(testVideoTrack.ClockRate)/int64(time.Second),
		[][]byte{
			testSPS,
			{5}, // IDR
		})
	require.EqualError(t, err, "reached maximum segment size")
}

func TestMuxerDoubleRead(t *testing.T) {
	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(testVideoTrack, testTime, 0, [][]byte{
		testSPS,
		{5}, // IDR
		{1},
	})
	require.NoError(t, err)

	err = m.WriteH264(
		testVideoTrack,
		testTime,
		int64(2*time.Second)*int64(testVideoTrack.ClockRate)/int64(time.Second),
		[][]byte{
			{5}, // IDR
			{2},
		})
	require.NoError(t, err)

	byts, _, err := doRequest(m, "main_stream.m3u8")
	require.NoError(t, err)

	re := regexp.MustCompile(`^#EXTM3U\n` +
		`#EXT-X-VERSION:3\n` +
		`#EXT-X-ALLOW-CACHE:NO\n` +
		`#EXT-X-TARGETDURATION:2\n` +
		`#EXT-X-MEDIA-SEQUENCE:0\n` +
		`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
		`#EXTINF:2.00000,\n` +
		`(.*?_seg0\.ts)\n$`)
	require.Regexp(t, re, string(byts))
	ma := re.FindStringSubmatch(string(byts))

	byts1, _, err := doRequest(m, ma[2])
	require.NoError(t, err)

	byts2, _, err := doRequest(m, ma[2])
	require.NoError(t, err)
	require.Equal(t, byts1, byts2)
}

func TestMuxerSaveToDisk(t *testing.T) {
	for _, ca := range []string{
		"mpegts",
		"mp4",
	} {
		t.Run(ca, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "gohlslib")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			var v MuxerVariant
			if ca == "mpegts" {
				v = MuxerVariantMPEGTS
			} else {
				v = MuxerVariantFMP4
			}

			m := &Muxer{
				Variant:            v,
				SegmentCount:       3,
				SegmentMinDuration: 1 * time.Second,
				Tracks:             []*Track{testVideoTrack},
				Directory:          dir,
			}

			err = m.Start()
			require.NoError(t, err)

			err = m.WriteH264(testVideoTrack, testTime, 0, [][]byte{
				testSPS,
				{5}, // IDR
				{1},
			})
			require.NoError(t, err)

			err = m.WriteH264(testVideoTrack, testTime, 2*90000, [][]byte{
				{5}, // IDR
				{2},
			})
			require.NoError(t, err)

			err = m.WriteH264(testVideoTrack, testTime, 3*90000, [][]byte{
				{5}, // IDR
				{2},
			})
			require.NoError(t, err)

			var u string
			if ca == "mpegts" {
				u = "main_stream.m3u8"
			} else {
				u = "video1_stream.m3u8"
			}

			byts, _, err := doRequest(m, u)
			require.NoError(t, err)

			var re *regexp.Regexp
			if ca == "mpegts" {
				re = regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:3\n` +
					`#EXT-X-ALLOW-CACHE:NO\n` +
					`#EXT-X-TARGETDURATION:2\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:2.00000,\n` +
					`(.*?_seg0\.ts)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(.*?_seg1\.ts)\n$`)
			} else {
				re = regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:10\n` +
					`#EXT-X-TARGETDURATION:2\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-MAP:URI="(.*?_init.mp4)"\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:2.00000,\n` +
					`(.*?_seg0\.mp4)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(.*?_seg1\.mp4)\n$`)
			}
			require.Regexp(t, re, string(byts))
			ma := re.FindStringSubmatch(string(byts))

			if ca == "mpegts" {
				_, err = os.ReadFile(filepath.Join(dir, ma[2]))
				require.NoError(t, err)

				m.Close()

				_, err = os.ReadFile(filepath.Join(dir, ma[2]))
				require.Error(t, err)
			} else {
				_, err = os.ReadFile(filepath.Join(dir, ma[3]))
				require.NoError(t, err)

				m.Close()

				_, err = os.ReadFile(filepath.Join(dir, ma[3]))
				require.Error(t, err)
			}
		})
	}
}

func TestMuxerDynamicParams(t *testing.T) {
	m := &Muxer{
		Variant:            MuxerVariantFMP4,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(testVideoTrack, testTime, 0, [][]byte{
		testSPS,
		{5}, // IDR
		{1},
	})
	require.NoError(t, err)

	err = m.WriteH264(testVideoTrack, testTime, 1*90000, [][]byte{
		{5}, // IDR
		{2},
	})
	require.NoError(t, err)

	err = m.WriteH264(testVideoTrack, testTime, 2*90000, [][]byte{
		{5}, // IDR
		{2},
	})
	require.NoError(t, err)

	bu, _, err := doRequest(m, "index.m3u8")
	require.NoError(t, err)
	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:9\n"+
		"#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=1144,AVERAGE-BANDWIDTH=1028,"+
		"CODECS=\"avc1.42c028\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
		"video1_stream.m3u8\n", string(bu))

	byts, _, err := doRequest(m, "video1_stream.m3u8")
	require.NoError(t, err)
	re := regexp.MustCompile(`^#EXTM3U\n` +
		`#EXT-X-VERSION:10\n` +
		`#EXT-X-TARGETDURATION:1\n` +
		`#EXT-X-MEDIA-SEQUENCE:0\n` +
		`#EXT-X-MAP:URI="(.*?_init.mp4)"\n` +
		`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
		`#EXTINF:1.00000,\n` +
		`(.*?_seg0\.mp4)\n` +
		`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
		`#EXTINF:1.00000,\n` +
		`(.*?_seg1\.mp4)\n$`)
	require.Regexp(t, re, string(byts))
	ma := re.FindStringSubmatch(string(byts))

	bu, _, err = doRequest(m, ma[1])
	require.NoError(t, err)

	func() {
		var init fmp4.Init
		err = init.Unmarshal(bytes.NewReader(bu))
		require.NoError(t, err)
		require.Equal(t, testSPS, init.Tracks[0].Codec.(*fmp4.CodecH264).SPS)
	}()

	// SPS (720p)
	testSPS2 := []byte{
		0x67, 0x64, 0x00, 0x1f, 0xac, 0xd9, 0x40, 0x50,
		0x05, 0xbb, 0x01, 0x6c, 0x80, 0x00, 0x00, 0x03,
		0x00, 0x80, 0x00, 0x00, 0x1e, 0x07, 0x8c, 0x18,
		0xcb,
	}

	err = m.WriteH264(testVideoTrack, testTime, 3*90000, [][]byte{
		testSPS2,
		{0x65, 0x88, 0x84, 0x00, 0x33, 0xff}, // IDR
		{2},
	})
	require.NoError(t, err)

	err = m.WriteH264(testVideoTrack, testTime, 5*90000, [][]byte{
		{0x65, 0x88, 0x84, 0x00, 0x33, 0xff}, // IDR
	})
	require.NoError(t, err)

	bu, _, err = doRequest(m, "index.m3u8")
	require.NoError(t, err)
	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:9\n"+
		"#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=912,AVERAGE-BANDWIDTH=752,"+
		"CODECS=\"avc1.64001f\",RESOLUTION=1280x720,FRAME-RATE=30.000\n"+
		"video1_stream.m3u8\n", string(bu))

	byts, _, err = doRequest(m, "video1_stream.m3u8")
	require.NoError(t, err)
	re = regexp.MustCompile(`^#EXTM3U\n` +
		`#EXT-X-VERSION:10\n` +
		`#EXT-X-TARGETDURATION:2\n` +
		`#EXT-X-MEDIA-SEQUENCE:1\n` +
		`#EXT-X-MAP:URI="(.*?_init.mp4)"\n` +
		`#EXTINF:1.00000,\n` +
		`(.*?_seg1\.mp4)\n` +
		`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
		`#EXTINF:1.00000,\n` +
		`(.*?_seg2\.mp4)\n` +
		`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
		`#EXTINF:2.00000,\n` +
		`(.*?_seg3\.mp4)\n$`)
	require.Regexp(t, re, string(byts))
	ma = re.FindStringSubmatch(string(byts))

	bu, _, err = doRequest(m, ma[1])
	require.NoError(t, err)

	var init fmp4.Init
	err = init.Unmarshal(bytes.NewReader(bu))
	require.NoError(t, err)
	require.Equal(t, testSPS2, init.Tracks[0].Codec.(*fmp4.CodecH264).SPS)
}

func TestMuxerFMP4ZeroDuration(t *testing.T) {
	m := &Muxer{
		Variant:            MuxerVariantFMP4,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(testVideoTrack, time.Now(), 0, [][]byte{
		testSPS, // SPS
		{8},     // PPS
		{5},     // IDR
	})
	require.NoError(t, err)

	err = m.WriteH264(testVideoTrack, time.Now(), 1, [][]byte{
		testSPS, // SPS
		{8},     // PPS
		{5},     // IDR
	})
	require.NoError(t, err)
}

func TestMuxerFMP4NegativeTimestamp(t *testing.T) {
	m := &Muxer{
		Variant:            MuxerVariantFMP4,
		SegmentCount:       3,
		SegmentMinDuration: 2 * time.Second,
		Tracks:             []*Track{testVideoTrack, testAudioTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteMPEG4Audio(testAudioTrack, testTime,
		-9*44100,
		[][]byte{
			{1, 2, 3, 4},
		})
	require.NoError(t, err)

	// this is skipped
	err = m.WriteH264(testVideoTrack, testTime,
		-11*90000,
		[][]byte{
			testSPS,
			{5}, // IDR
			{1},
		})
	require.Error(t, err, "sample timestamp is impossible to handle")
}

func TestMuxerFMP4SequenceNumber(t *testing.T) {
	m := &Muxer{
		Variant:            MuxerVariantLowLatency,
		SegmentCount:       7,
		SegmentMinDuration: 2 * time.Second,
		Tracks:             []*Track{testVideoTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(testVideoTrack, testTime, 0, [][]byte{
		testSPS,
		{5}, // IDR
		{1},
	})
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		err = m.WriteH264(testVideoTrack, testTime,
			(1+int64(i))*90000, [][]byte{
				{1}, // non IDR
			})
		require.NoError(t, err)
	}

	err = m.WriteH264(testVideoTrack, testTime,
		4*90000,
		[][]byte{
			{5}, // IDR
		})
	require.NoError(t, err)

	byts, _, err := doRequest(m, "index.m3u8")
	require.NoError(t, err)
	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:9\n"+
		"#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=964,AVERAGE-BANDWIDTH=964,"+
		"CODECS=\"avc1.42c028\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
		"video1_stream.m3u8\n", string(byts))

	byts, _, err = doRequest(m, "video1_stream.m3u8")
	require.NoError(t, err)
	re := regexp.MustCompile(`^#EXTM3U\n` +
		`#EXT-X-VERSION:10\n` +
		`#EXT-X-TARGETDURATION:4\n` +
		`#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=2\.50000,CAN-SKIP-UNTIL=24\.00000\n` +
		`#EXT-X-PART-INF:PART-TARGET=1.00000\n` +
		`#EXT-X-MEDIA-SEQUENCE:1\n` +
		`#EXT-X-MAP:URI=".*?_init.mp4"\n` +
		`#EXT-X-GAP\n` +
		`#EXTINF:4.00000,\n` +
		`gap.mp4\n` +
		`#EXT-X-GAP\n` +
		`#EXTINF:4.00000,\n` +
		`gap.mp4\n` +
		`#EXT-X-GAP\n` +
		`#EXTINF:4.00000,\n` +
		`gap.mp4\n` +
		`#EXT-X-GAP\n` +
		`#EXTINF:4.00000,\n` +
		`gap.mp4\n` +
		`#EXT-X-GAP\n` +
		`#EXTINF:4.00000,\n` +
		`gap.mp4\n` +
		`#EXT-X-GAP\n` +
		`#EXTINF:4.00000,\n` +
		`gap.mp4\n` +
		`#EXT-X-PROGRAM-DATE-TIME:.*?\n` +
		`#EXT-X-PART:DURATION=1.00000,URI="(.*?)_part0.mp4",INDEPENDENT=YES\n` +
		`#EXT-X-PART:DURATION=1.00000,URI=".*?_part1.mp4"\n` +
		`#EXT-X-PART:DURATION=1.00000,URI=".*?_part2.mp4"\n` +
		`#EXT-X-PART:DURATION=1.00000,URI=".*?_part3.mp4"\n` +
		`#EXTINF:4.00000,\n` +
		`(.*?_seg7\.mp4)\n` +
		`#EXT-X-PRELOAD-HINT:TYPE=PART,URI=".*?_part4.mp4"\n$`)
	require.Regexp(t, re, string(byts))
	ma := re.FindStringSubmatch(string(byts))

	for i := 0; i < 3; i++ {
		buf, _, err := doRequest(m, ma[1]+"_part"+strconv.FormatInt(int64(i), 10)+".mp4")
		require.NoError(t, err)

		var parts fmp4.Parts
		err = parts.Unmarshal(buf)
		require.NoError(t, err)
		require.Equal(t, uint32(i), parts[0].SequenceNumber)
	}
}

func TestMuxerInvalidFolder(t *testing.T) {
	for _, ca := range []string{
		"mpegts",
		"fmp4",
	} {
		t.Run(ca, func(t *testing.T) {
			var v MuxerVariant
			if ca == "mpegts" {
				v = MuxerVariantMPEGTS
			} else {
				v = MuxerVariantFMP4
			}

			m := &Muxer{
				Variant:            v,
				SegmentCount:       7,
				SegmentMinDuration: 1 * time.Second,
				Tracks:             []*Track{testVideoTrack},
				Directory:          "/nonexisting",
			}

			err := m.Start()
			require.NoError(t, err)
			defer m.Close()

			for i := 0; i < 2; i++ {
				err := m.WriteH264(testVideoTrack, testTime,
					int64(i)*90000,
					[][]byte{
						testSPS, // SPS
						{8},     // PPS
						{5},     // IDR
					})

				if ca == "mpegts" || i == 1 {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}

func TestMuxerExpiredSegment(t *testing.T) {
	m := &Muxer{
		Variant:            MuxerVariantLowLatency,
		SegmentCount:       7,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := 0; i < 2; i++ {
		err = m.WriteH264(testVideoTrack, testTime,
			int64(i)*90000,
			[][]byte{
				testSPS, // SPS
				{8},     // PPS
				{5},     // IDR
			})
		require.NoError(t, err)
	}

	byts, _, err := doRequest(m, "index.m3u8")
	require.NoError(t, err)
	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:9\n"+
		"#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=1144,AVERAGE-BANDWIDTH=1144,"+
		"CODECS=\"avc1.42c028\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
		"video1_stream.m3u8\n", string(byts))

	v := url.Values{}
	v.Set("_HLS_msn", "1")
	v.Set("_HLS_part", "0")

	r := &http.Request{
		URL: &url.URL{
			Path:     "video1_stream.m3u8",
			RawQuery: v.Encode(),
		},
	}

	w := &dummyResponseWriter{
		h: make(http.Header),
	}

	m.Handle(w, r)
	require.Equal(t, http.StatusBadRequest, w.statusCode)
}

func TestMuxerPreloadHint(t *testing.T) {
	m := &Muxer{
		Variant:            MuxerVariantLowLatency,
		SegmentCount:       7,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := 0; i < 2; i++ {
		err2 := m.WriteH264(testVideoTrack, testTime,
			int64(i)*90000,
			[][]byte{
				testSPS, // SPS
				{8},     // PPS
				{5},     // IDR
			})
		require.NoError(t, err2)
	}

	byts, _, err := doRequest(m, "index.m3u8")
	require.NoError(t, err)
	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:9\n"+
		"#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=1144,AVERAGE-BANDWIDTH=1144,"+
		"CODECS=\"avc1.42c028\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
		"video1_stream.m3u8\n", string(byts))

	byts, _, err = doRequest(m, "video1_stream.m3u8")
	require.NoError(t, err)

	re := regexp.MustCompile(`^#EXTM3U\n` +
		`#EXT-X-VERSION:10\n` +
		`#EXT-X-TARGETDURATION:1\n` +
		`#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=2.50000,CAN-SKIP-UNTIL=6.00000\n` +
		`#EXT-X-PART-INF:PART-TARGET=1.00000\n` +
		`#EXT-X-MEDIA-SEQUENCE:1\n` +
		`#EXT-X-MAP:URI=".*?_init\.mp4"\n` +
		`#EXT-X-GAP\n` +
		`#EXTINF:1.00000,\n` +
		`gap.mp4\n` +
		`#EXT-X-GAP\n` +
		`#EXTINF:1.00000,\n` +
		`gap.mp4\n` +
		`#EXT-X-GAP\n` +
		`#EXTINF:1.00000,\n` +
		`gap.mp4\n` +
		`#EXT-X-GAP\n` +
		`#EXTINF:1.00000,\n` +
		`gap.mp4\n` +
		`#EXT-X-GAP\n` +
		`#EXTINF:1.00000,\n` +
		`gap.mp4\n` +
		`#EXT-X-GAP\n` +
		`#EXTINF:1.00000,\n` +
		`gap.mp4\n` +
		`#EXT-X-PROGRAM-DATE-TIME:.*?\n` +
		`#EXT-X-PART:DURATION=1.00000,URI=".*?_part0\.mp4",INDEPENDENT=YES\n` +
		`#EXTINF:1.00000,\n` +
		`.*?_seg7\.mp4\n` +
		`#EXT-X-PRELOAD-HINT:TYPE=PART,URI="(.*?_part1\.mp4)"\n$`)
	require.Regexp(t, re, string(byts))
	ma := re.FindStringSubmatch(string(byts))

	preloadDone := make(chan []byte)

	go func() {
		byts2, _, err2 := doRequest(m, ma[1])
		require.NoError(t, err2)
		preloadDone <- byts2
	}()

	select {
	case <-preloadDone:
		t.Error("should not happen")
	case <-time.After(500 * time.Millisecond):
	}

	err = m.WriteH264(testVideoTrack, testTime,
		3*90000,
		[][]byte{
			{5}, // IDR
		})
	require.NoError(t, err)

	byts = <-preloadDone

	var parts fmp4.Parts
	err = parts.Unmarshal(byts)
	require.NoError(t, err)

	require.Equal(t, fmp4.Parts{{
		SequenceNumber: 1,
		Tracks: []*fmp4.PartTrack{{
			ID:       1,
			BaseTime: 990000,
			Samples: []*fmp4.PartSample{{
				Duration: 180000,
				Payload: []byte{
					0x00, 0x00, 0x00, 0x19, 0x67, 0x42, 0xc0, 0x28,
					0xd9, 0x00, 0x78, 0x02, 0x27, 0xe5, 0x84, 0x00,
					0x00, 0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00,
					0xf0, 0x3c, 0x60, 0xc9, 0x20, 0x00, 0x00, 0x00,
					0x01, 0x08, 0x00, 0x00, 0x00, 0x01, 0x05,
				},
			}},
		}},
	}}, parts)
}
