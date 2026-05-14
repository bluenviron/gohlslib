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
	mp4codecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
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

var testConfig = mpeg4audio.AudioSpecificConfig{
	Type:          2,
	SampleRate:    44100,
	ChannelConfig: 2,
	ChannelCount:  2,
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
		Config: mpeg4audio.AudioSpecificConfig{
			Type:          2,
			SampleRate:    44100,
			ChannelConfig: 2,
			ChannelCount:  2,
		},
	},
	ClockRate: 44100,
}

var testAudioTrack2 = &Track{
	Codec: &codecs.MPEG4Audio{
		Config: mpeg4audio.AudioSpecificConfig{
			Type:          2,
			SampleRate:    44100,
			ChannelConfig: 2,
			ChannelCount:  2,
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
			for i := range 100 {
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
			for i := range 100 {
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
		require.Equal(t, "public, max-age=30", h.Get("Cache-Control"))

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

		switch variant {
		case "lowLatency":
			require.Equal(t, "no-cache", h.Get("Cache-Control"))

		case "mpegts":
			if content == "audio" {
				require.Equal(t, "public, max-age=2", h.Get("Cache-Control"))
			} else {
				require.Equal(t, "public, max-age=1", h.Get("Cache-Control"))
			}

		default:
			require.Equal(t, "public, max-age=1", h.Get("Cache-Control"))
		}

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
			ma := re.FindStringSubmatch(string(byts))

			_, h, err = doRequest(m, ma[1])
			require.NoError(t, err)
			require.Equal(t, "video/mp2t", h.Get("Content-Type"))
			require.Equal(t, "public, max-age=3600", h.Get("Cache-Control"))

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
				`#EXT-X-PROGRAM-DATE-TIME:.*?\n` +
				`#EXTINF:4.00000,\n` +
				`(.*?_seg0\.mp4\?key=value)\n` +
				`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
				`#EXTINF:1.00000,\n` +
				`(.*?_seg1\.mp4\?key=value)\n$`)
			require.Regexp(t, re, string(byts))
			ma := re.FindStringSubmatch(string(byts))

			// init
			_, h, err = doRequest(m, ma[1])
			require.NoError(t, err)
			require.Equal(t, "video/mp4", h.Get("Content-Type"))
			require.Equal(t, "public, max-age=3600", h.Get("Cache-Control"))

			// segment 1
			_, h, err = doRequest(m, ma[2])
			require.NoError(t, err)
			require.Equal(t, "video/mp4", h.Get("Content-Type"))
			require.Equal(t, "public, max-age=3600", h.Get("Cache-Control"))

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
			ma := re.FindStringSubmatch(string(byts))

			// init
			_, h, err = doRequest(m, ma[1])
			require.NoError(t, err)
			require.Equal(t, "audio/mp4", h.Get("Content-Type"))
			require.Equal(t, "public, max-age=3600", h.Get("Cache-Control"))

			// segment 1
			_, h, err = doRequest(m, ma[2])
			require.NoError(t, err)
			require.Equal(t, "audio/mp4", h.Get("Content-Type"))
			require.Equal(t, "public, max-age=3600", h.Get("Cache-Control"))

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

		switch variant {
		case "lowLatency":
			require.Equal(t, "no-cache", h.Get("Cache-Control"))

		default:
			require.Equal(t, "public, max-age=1", h.Get("Cache-Control"))
		}

		switch variant {
		case "fmp4":
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

		case "lowLatency":
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

func TestMuxerCloseWhileRequestStream(t *testing.T) {
	m := &Muxer{
		Variant:            MuxerVariantFMP4,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack},
	}

	err := m.Start()
	require.NoError(t, err)

	done := make(chan struct{})

	go func() {
		b, _, _ := doRequest(m, "video1_stream.m3u8")
		require.Equal(t, []byte(nil), b)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)

	m.Close()
	<-done
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

			var mediaPlaylistName string
			if ca == "mpegts" {
				mediaPlaylistName = "main_stream.m3u8"
			} else {
				mediaPlaylistName = "video1_stream.m3u8"
			}

			diskMultivariantByts, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
			require.NoError(t, err)

			diskMediaPlaylistByts, err := os.ReadFile(filepath.Join(dir, mediaPlaylistName))
			require.NoError(t, err)

			if ca == "mp4" {
				re := regexp.MustCompile(`#EXT-X-MAP:URI="(.*?_init.mp4)"`)
				ma := re.FindStringSubmatch(string(diskMediaPlaylistByts))
				require.Len(t, ma, 2)

				_, err = os.ReadFile(filepath.Join(dir, ma[1]))
				require.NoError(t, err)
			}

			multivariantByts, _, err := doRequest(m, "index.m3u8")
			require.NoError(t, err)

			byts, _, err := doRequest(m, mediaPlaylistName)
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

			require.Equal(t, multivariantByts, diskMultivariantByts)
			require.Equal(t, byts, diskMediaPlaylistByts)

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

	index1, _, err := doRequest(m, "index.m3u8")
	require.NoError(t, err)
	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:9\n"+
		"#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=1144,AVERAGE-BANDWIDTH=1028,"+
		"CODECS=\"avc1.42c028\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
		"video1_stream.m3u8\n", string(index1))

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

	bu, _, err := doRequest(m, ma[1])
	require.NoError(t, err)

	var init1 fmp4.Init
	err = init1.Unmarshal(bytes.NewReader(bu))
	require.NoError(t, err)
	require.Equal(t, testSPS, init1.Tracks[0].Codec.(*mp4codecs.H264).SPS)

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

	index2, _, err := doRequest(m, "index.m3u8")
	require.NoError(t, err)
	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:9\n"+
		"#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=912,AVERAGE-BANDWIDTH=752,"+
		"CODECS=\"avc1.42c028\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
		"video1_stream.m3u8\n", string(index2))

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

	var init2 fmp4.Init
	err = init2.Unmarshal(bytes.NewReader(bu))
	require.NoError(t, err)
	require.Equal(t, init1, init2)
}

func TestMuxerInStreamParams(t *testing.T) {
	getInit := func(t *testing.T, m *Muxer) *fmp4.Init {
		byts, _, err := doRequest(m, "video1_stream.m3u8")
		require.NoError(t, err)
		re := regexp.MustCompile(`#EXT-X-MAP:URI="(.*?)"`)
		ma := re.FindStringSubmatch(string(byts))
		require.Len(t, ma, 2)
		bu, _, err := doRequest(m, ma[1])
		require.NoError(t, err)
		var init fmp4.Init
		err = init.Unmarshal(bytes.NewReader(bu))
		require.NoError(t, err)
		return &init
	}

	for _, ca := range []string{
		"h264",
		"h265",
		"av1",
		"vp9",
	} {
		t.Run(ca, func(t *testing.T) {
			var track *Track

			switch ca {
			case "h264":
				track = &Track{
					Codec: &codecs.H264{
						SPS: []byte{0x67, 0x42, 0xc0, 0x0a}, // different initial SPS
						PPS: []byte{0x00},                   // different initial PPS
					},
					ClockRate: 90000,
				}

			case "h265":
				track = &Track{
					Codec: &codecs.H265{
						VPS: []byte{0x40, 0x01}, // wrong initial params
						SPS: []byte{0x42, 0x01},
						PPS: []byte{0x44, 0x01},
					},
					ClockRate: 90000,
				}

			case "av1":
				track = &Track{
					Codec:     &codecs.AV1{SequenceHeader: []byte{0x08}}, // different initial (empty payload)
					ClockRate: 90000,
				}

			case "vp9":
				track = &Track{
					Codec:     &codecs.VP9{}, // different initial (zero values)
					ClockRate: 90000,
				}
			}

			m := &Muxer{
				Variant:            MuxerVariantFMP4,
				SegmentCount:       3,
				SegmentMinDuration: 1 * time.Second,
				Tracks:             []*Track{track},
			}
			err := m.Start()
			require.NoError(t, err)
			defer m.Close()

			switch ca {
			case "h264":
				inStreamPPS := []byte{0x08, 0xDE, 0xAD}

				err = m.WriteH264(track, testTime, 0, [][]byte{testSPS, inStreamPPS, {5}})
				require.NoError(t, err)

				err = m.WriteH264(track, testTime, 2*90000, [][]byte{{5}})
				require.NoError(t, err)

				err = m.WriteH264(track, testTime, 4*90000, [][]byte{{5}})
				require.NoError(t, err)

				h264Codec := getInit(t, m).Tracks[0].Codec.(*mp4codecs.H264)
				require.Equal(t, testSPS, h264Codec.SPS)
				require.Equal(t, inStreamPPS, h264Codec.PPS)

			case "h265":
				inStreamVPS := []byte{
					0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x01, 0x60,
					0x00, 0x00, 0x03, 0x00, 0x90, 0x00, 0x00, 0x03,
					0x00, 0x00, 0x03, 0x00, 0x78, 0x99, 0x98, 0x09,
				}
				inStreamSPS := []byte{
					0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x03,
					0x00, 0x90, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03,
					0x00, 0x78, 0xa0, 0x03, 0xc0, 0x80, 0x10, 0xe5,
					0x96, 0x66, 0x69, 0x24, 0xca, 0xe0, 0x10, 0x00,
					0x00, 0x03, 0x00, 0x10, 0x00, 0x00, 0x03, 0x01,
					0xe0, 0x80,
				}
				inStreamPPS := []byte{0x44, 0x01, 0xc1, 0x72, 0xb4, 0x62, 0x40}
				idrNALU := []byte{0x26, 0x01, 0xaf, 0x08, 0x42, 0x23, 0x48, 0x8a, 0x43, 0xe2}

				err = m.WriteH265(track, testTime, 0, [][]byte{inStreamVPS, inStreamSPS, inStreamPPS, idrNALU})
				require.NoError(t, err)

				err = m.WriteH265(track, testTime, 2*90000, [][]byte{idrNALU})
				require.NoError(t, err)

				err = m.WriteH265(track, testTime, 4*90000, [][]byte{idrNALU})
				require.NoError(t, err)

				err = m.WriteH265(track, testTime, 6*90000, [][]byte{idrNALU})
				require.NoError(t, err)

				h265Codec := getInit(t, m).Tracks[0].Codec.(*mp4codecs.H265)
				require.Equal(t, inStreamVPS, h265Codec.VPS)
				require.Equal(t, inStreamSPS, h265Codec.SPS)
				require.Equal(t, inStreamPPS, h265Codec.PPS)

			case "av1":
				inStreamSeqHeader := []byte{
					0x08, 0x00, 0x00, 0x00, 0x42, 0xa7, 0xbf, 0xe4,
					0x60, 0x0d, 0x00, 0x40,
				}

				err = m.WriteAV1(track, testTime, 0, [][]byte{inStreamSeqHeader})
				require.NoError(t, err)

				err = m.WriteAV1(track, testTime, 2*90000, [][]byte{inStreamSeqHeader})
				require.NoError(t, err)

				err = m.WriteAV1(track, testTime, 4*90000, [][]byte{inStreamSeqHeader})
				require.NoError(t, err)

				av1Codec := getInit(t, m).Tracks[0].Codec.(*mp4codecs.AV1)
				require.Equal(t, inStreamSeqHeader, av1Codec.SequenceHeader)

			case "vp9":
				// valid VP9 key frame (chrome webrtc, 1920x804, profile 0)
				keyFrame := []byte{
					0x82, 0x49, 0x83, 0x42, 0x00, 0x77, 0xf0, 0x32,
					0x34, 0x30, 0x38, 0x24, 0x1c, 0x19, 0x40, 0x18,
					0x03, 0x40, 0x5f, 0xb4,
				}

				err = m.WriteVP9(track, testTime, 0, keyFrame)
				require.NoError(t, err)

				err = m.WriteVP9(track, testTime, 2*90000, keyFrame)
				require.NoError(t, err)

				err = m.WriteVP9(track, testTime, 4*90000, keyFrame)
				require.NoError(t, err)

				vp9Codec := getInit(t, m).Tracks[0].Codec.(*mp4codecs.VP9)
				require.Equal(t, 1920, vp9Codec.Width)
				require.Equal(t, 804, vp9Codec.Height)
				require.Equal(t, uint8(0), vp9Codec.Profile)
				require.Equal(t, uint8(8), vp9Codec.BitDepth)
				require.Equal(t, uint8(1), vp9Codec.ChromaSubsampling)
				require.Equal(t, false, vp9Codec.ColorRange)
			}
		})
	}
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

func TestMuxerFMP4NegativeInitialDTS(t *testing.T) {
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

func TestMuxerFMP4NegativeDTSDiff(t *testing.T) {
	m := &Muxer{
		Variant:            MuxerVariantFMP4,
		SegmentCount:       3,
		SegmentMinDuration: 2 * time.Second,
		Tracks:             []*Track{testAudioTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteMPEG4Audio(testAudioTrack, testTime,
		1*44100,
		[][]byte{{1, 2}})
	require.NoError(t, err)

	err = m.WriteMPEG4Audio(testAudioTrack, testTime,
		3*44100,
		[][]byte{{1, 2}})
	require.NoError(t, err)

	err = m.WriteMPEG4Audio(testAudioTrack, testTime,
		2*44100,
		[][]byte{{1, 2}})
	require.NoError(t, err)

	for i := 4; i < 10; i++ {
		err = m.WriteMPEG4Audio(testAudioTrack, testTime,
			int64(i)*44100,
			[][]byte{{1, 2}})
		require.NoError(t, err)
	}

	byts, _, err := doRequest(m, "audio1_stream.m3u8")
	require.NoError(t, err)
	require.Equal(t, `#EXTM3U`+"\n"+
		`#EXT-X-VERSION:10`+"\n"+
		`#EXT-X-TARGETDURATION:2`+"\n"+
		`#EXT-X-MEDIA-SEQUENCE:1`+"\n"+
		`#EXT-X-MAP:URI="`+m.prefix+`_audio1_init.mp4"`+"\n"+
		`#EXTINF:2.00000,`+"\n"+
		``+m.prefix+`_audio1_seg1.mp4`+"\n"+
		`#EXT-X-PROGRAM-DATE-TIME:2010-01-01T01:01:01Z`+"\n"+
		`#EXTINF:2.00000,`+"\n"+
		``+m.prefix+`_audio1_seg2.mp4`+"\n"+
		`#EXT-X-PROGRAM-DATE-TIME:2010-01-01T01:01:01Z`+"\n"+
		`#EXTINF:2.00000,`+"\n"+
		``+m.prefix+`_audio1_seg3.mp4`+"\n",
		string(byts))

	byts, _, err = doRequest(m, m.prefix+"_audio1_seg1.mp4")
	require.NoError(t, err)

	var parts fmp4.Parts
	err = parts.Unmarshal(byts)
	require.NoError(t, err)

	require.Equal(t, fmp4.Parts{{
		SequenceNumber: 1,
		Tracks: []*fmp4.PartTrack{{
			ID:       1,
			BaseTime: 573300,
			Samples: []*fmp4.Sample{
				{
					Payload: []byte{1, 2},
				},
				{
					Duration: 44100,
					Payload:  []byte{1, 2},
				},
				{
					Duration: 44100,
					Payload:  []byte{1, 2},
				},
			},
		}},
	}}, parts)
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

	for i := range 3 {
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

	for i := range 3 {
		var buf []byte
		buf, _, err = doRequest(m, ma[1]+"_part"+strconv.FormatInt(int64(i), 10)+".mp4")
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

			for i := range 2 {
				err = m.WriteH264(testVideoTrack, testTime,
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

	for i := range 2 {
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

	for i := range 2 {
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
			Samples: []*fmp4.Sample{{
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
