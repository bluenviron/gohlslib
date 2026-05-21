package gohlslib

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/asticode/go-astits"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/flac"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mp4codecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

var testTime = time.Date(2010, 0o1, 0o1, 0o1, 0o1, 0o1, 0, time.UTC)

// baseline profile without POC
var testH264SPS = []byte{
	0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
	0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
	0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
	0x20,
}

var testH264PPS = []byte{0x01, 0x02, 0x03, 0x04}

var testH265VPS = []byte{
	0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x01, 0x60,
	0x00, 0x00, 0x03, 0x00, 0x90, 0x00, 0x00, 0x03,
	0x00, 0x00, 0x03, 0x00, 0x78, 0x99, 0x98, 0x09,
}

var testH265SPS = []byte{
	0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x03,
	0x00, 0x90, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03,
	0x00, 0x78, 0xa0, 0x03, 0xc0, 0x80, 0x10, 0xe5,
	0x96, 0x66, 0x69, 0x24, 0xca, 0xe0, 0x10, 0x00,
	0x00, 0x03, 0x00, 0x10, 0x00, 0x00, 0x03, 0x01,
	0xe0, 0x80,
}

var testH265PPS = []byte{0x44, 0x01, 0xc1, 0x72, 0xb4, 0x62, 0x40}

var testAV1SequenceHeader = []byte{8, 0, 0, 0, 66, 167, 191, 230, 46, 223, 200, 66}

var testVP9KeyFrame = []byte{
	0x82, 0x49, 0x83, 0x42, 0x00, 0x77, 0xf0, 0x32,
	0x34, 0x30, 0x38, 0x24, 0x1c, 0x19, 0x40, 0x18,
	0x03, 0x40, 0x5f, 0xb4,
}

var testAACConfig = mpeg4audio.AudioSpecificConfig{
	Type:          2,
	SampleRate:    44100,
	ChannelConfig: 2,
	ChannelCount:  2,
}

var testVideoTrack = &Track{
	Codec: &codecs.H264{
		SPS: testH264SPS,
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
					testH264SPS, // SPS
					{8},         // PPS
					{5},         // IDR
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
					testH264SPS, // SPS
					{8},         // PPS
					{5},         // IDR
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
					testH264SPS, // SPS
					{8},         // PPS
					{5},         // IDR
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

func TestMuxerCodecs(t *testing.T) {
	for _, ca := range []string{
		"h264",
		"h265",
		"av1",
		"vp9",
		"opus",
		"mpeg4_audio",
		"flac",
	} {
		t.Run(ca, func(t *testing.T) {
			var track *Track
			switch ca {
			case "h264":
				track = &Track{
					Codec:     &codecs.H264{SPS: testH264SPS, PPS: []byte{0x08}},
					ClockRate: 90000,
				}
			case "h265":
				track = &Track{
					Codec:     &codecs.H265{VPS: testH265VPS, SPS: testH265SPS, PPS: testH265PPS},
					ClockRate: 90000,
				}
			case "av1":
				track = &Track{
					Codec:     &codecs.AV1{SequenceHeader: testAV1SequenceHeader},
					ClockRate: 90000,
				}
			case "vp9":
				track = &Track{
					Codec:     &codecs.VP9{},
					ClockRate: 90000,
				}
			case "opus":
				track = &Track{
					Codec:     &codecs.Opus{ChannelCount: 2},
					ClockRate: 48000,
				}
			case "mpeg4_audio":
				track = &Track{
					Codec:     &codecs.MPEG4Audio{Config: testAACConfig},
					ClockRate: 44100,
				}
			case "flac":
				track = &Track{
					Codec: &codecs.FLAC{
						StreamInfo: &flac.StreamInfo{
							MinBlockSize: 4096,
							MaxBlockSize: 4096,
							SampleRate:   44100,
							ChannelCount: 2,
							BitDepth:     16,
						},
					},
					ClockRate: 44100,
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
				require.NoError(t, m.WriteH264(track, testTime, 0, [][]byte{testH264SPS, {0x08}, {5}}))
				require.NoError(t, m.WriteH264(track, testTime.Add(time.Second), 90000, [][]byte{{5}}))
				require.NoError(t, m.WriteH264(track, testTime.Add(2*time.Second), 180000, [][]byte{{5}}))
			case "h265":
				require.NoError(t, m.WriteH265(track,
					testTime, 0, [][]byte{
						testH265VPS, testH265SPS, testH265PPS,
						{0x26, 0x01, 0xaf, 0x08, 0x42, 0x23, 0x48, 0x8a, 0x43, 0xe2},
					}))
				require.NoError(t, m.WriteH265(track,
					testTime.Add(3*time.Second), 3*90000,
					[][]byte{{0x26, 0x01, 0xaf, 0x08, 0x42, 0x23, 0x48, 0x8a, 0x43, 0xe2}}))
				require.NoError(t, m.WriteH265(track,
					testTime.Add(6*time.Second), 6*90000,
					[][]byte{{0x26, 0x01, 0xaf, 0x08, 0x42, 0x23, 0x48, 0x8a, 0x43, 0xe2}}))
				require.NoError(t, m.WriteH265(track,
					testTime.Add(9*time.Second), 9*90000,
					[][]byte{{0x26, 0x01, 0xaf, 0x08, 0x42, 0x23, 0x48, 0x8a, 0x43, 0xe2}}))
			case "av1":
				require.NoError(t, m.WriteAV1(track,
					testTime, 0, [][]byte{testAV1SequenceHeader}))
				require.NoError(t, m.WriteAV1(track,
					testTime.Add(time.Second), 90000, [][]byte{testAV1SequenceHeader}))
				require.NoError(t, m.WriteAV1(track,
					testTime.Add(2*time.Second), 180000, [][]byte{testAV1SequenceHeader}))
			case "vp9":
				require.NoError(t, m.WriteVP9(track, testTime, 0, testVP9KeyFrame))
				require.NoError(t, m.WriteVP9(track, testTime.Add(time.Second), 90000, testVP9KeyFrame))
				require.NoError(t, m.WriteVP9(track, testTime.Add(2*time.Second), 180000, testVP9KeyFrame))
			case "opus":
				require.NoError(t, m.WriteOpus(track, testTime, 0, [][]byte{{0xf8}}))
				require.NoError(t, m.WriteOpus(track, testTime.Add(time.Second), 48000, [][]byte{{0xf8}}))
				require.NoError(t, m.WriteOpus(track, testTime.Add(2*time.Second), 96000, [][]byte{{0xf8}}))
			case "mpeg4_audio":
				require.NoError(t, m.WriteMPEG4Audio(track, testTime, 0, [][]byte{{0x21, 0x10}}))
				require.NoError(t, m.WriteMPEG4Audio(track, testTime.Add(time.Second), 44100, [][]byte{{0x21, 0x10}}))
				require.NoError(t, m.WriteMPEG4Audio(track, testTime.Add(2*time.Second), 88200, [][]byte{{0x21, 0x10}}))
			case "flac":
				require.NoError(t, m.WriteFLAC(track,
					testTime, 0, []byte{0x00, 0x01, 0x02, 0x03}))
				require.NoError(t, m.WriteFLAC(track,
					testTime.Add(time.Second), 44100, []byte{0x00, 0x01, 0x02, 0x03}))
				require.NoError(t, m.WriteFLAC(track,
					testTime.Add(2*time.Second), 88200, []byte{0x00, 0x01, 0x02, 0x03}))
			}

			var codecStr string
			switch ca {
			case "h264":
				codecStr = "avc1.42c028"
			case "h265":
				codecStr = "hvc1.1.6.L120.90"
			case "av1":
				codecStr = "av01.0.08M.08.0.110.01.01.01.0"
			case "vp9":
				codecStr = "vp09.00.10.08"
			case "opus":
				codecStr = "opus"
			case "mpeg4_audio":
				codecStr = "mp4a.40.2"
			case "flac":
				codecStr = "flac"
			}

			byts, _, err := doRequest(m, "index.m3u8")
			require.NoError(t, err)
			require.Contains(t, string(byts), codecStr)

			re := regexp.MustCompile(`(\w+_stream\.m3u8)`)
			ma := re.FindStringSubmatch(string(byts))
			require.NotNil(t, ma)

			mediaPlaylist, _, err := doRequest(m, ma[1])
			require.NoError(t, err)

			re = regexp.MustCompile(`#EXT-X-MAP:URI="(.*?)"`)
			ma = re.FindStringSubmatch(string(mediaPlaylist))
			require.NotNil(t, ma)

			byts, _, err = doRequest(m, ma[1])
			require.NoError(t, err)

			var init fmp4.Init
			err = init.Unmarshal(bytes.NewReader(byts))
			require.NoError(t, err)
			require.Len(t, init.Tracks, 1)

			switch ca {
			case "h264":
				require.Equal(t, &mp4codecs.H264{
					SPS: testH264SPS,
					PPS: []byte{0x08},
				}, init.Tracks[0].Codec)
			case "h265":
				require.Equal(t, &mp4codecs.H265{
					VPS: testH265VPS,
					SPS: testH265SPS,
					PPS: testH265PPS,
				}, init.Tracks[0].Codec)
			case "av1":
				require.Equal(t, &mp4codecs.AV1{
					SequenceHeader: testAV1SequenceHeader,
				}, init.Tracks[0].Codec)
			case "vp9":
				require.Equal(t, &mp4codecs.VP9{
					Width:             1920,
					Height:            804,
					BitDepth:          8,
					ChromaSubsampling: 1,
				}, init.Tracks[0].Codec)
			case "opus":
				require.Equal(t, &mp4codecs.Opus{
					ChannelCount: 2,
				}, init.Tracks[0].Codec)
			case "mpeg4_audio":
				require.Equal(t, &mp4codecs.MPEG4Audio{
					Config: testAACConfig,
				}, init.Tracks[0].Codec)
			case "flac":
				require.Equal(t, &mp4codecs.FLAC{
					StreamInfo: &flac.StreamInfo{
						MinBlockSize: 4096,
						MaxBlockSize: 4096,
						SampleRate:   44100,
						ChannelCount: 2,
						BitDepth:     16,
					},
				}, init.Tracks[0].Codec)
			}

			re = regexp.MustCompile(`(.*?_seg0\.mp4)`)
			ma = re.FindStringSubmatch(string(mediaPlaylist))
			require.NotNil(t, ma)

			byts, _, err = doRequest(m, ma[1])
			require.NoError(t, err)
			require.NotEmpty(t, byts)
		})
	}
}

func TestMuxerKLV(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack, klvTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := range 4 {
		d := time.Duration(i) * time.Second
		pts := int64(d) * 90000 / int64(time.Second)

		err = m.WriteKLV(klvTrack, testTime.Add(d), pts, []byte{
			0x00, 0x01, 0x02, 0x03,
		})
		require.NoError(t, err)

		// Write H264 (IDR to force segment creation)
		err = m.WriteH264(testVideoTrack, testTime.Add(d), pts, [][]byte{
			testH264SPS,
			{8}, // PPS
			{5}, // IDR
		})
		require.NoError(t, err)
	}

	byts, _, err := doRequest(m, "index.m3u8")
	require.NoError(t, err)

	require.Contains(t, string(byts), "main_stream.m3u8")

	byts, _, err = doRequest(m, "main_stream.m3u8")
	require.NoError(t, err)

	re := regexp.MustCompile(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-ALLOW-CACHE:NO
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-PROGRAM-DATE-TIME:.*?
#EXTINF:1.00000,
(.*?_main_seg0\.ts)
#EXT-X-PROGRAM-DATE-TIME:.*?
#EXTINF:1.00000,
(.*?_main_seg1\.ts)
#EXT-X-PROGRAM-DATE-TIME:.*?
#EXTINF:1.00000,
(.*?_main_seg2\.ts)
`)
	require.Regexp(t, re, string(byts))
	ma := re.FindStringSubmatch(string(byts))

	// Fetch the first segment and parse it to verify KLV data
	segmentData, _, err := doRequest(m, ma[1])
	require.NoError(t, err)
	require.NotEmpty(t, segmentData)

	// Parse the MPEG-TS segment using astits
	demuxer := astits.NewDemuxer(context.Background(), bytes.NewReader(segmentData))

	// Track whether we found KLV data
	foundKLVPID := false
	foundKLVData := false
	var klvPID uint16
	var receivedKLVData []byte

	// Iterate through all packets in the segment
	for {
		data, nextErr := demuxer.NextData()
		if nextErr != nil {
			break
		}

		// Check if this is a PMT (Program Map Table) and find KLV PID
		if data.PMT != nil && !foundKLVPID {
			for _, es := range data.PMT.ElementaryStreams {
				// KLV data uses stream type 0x06 (private data)
				if es.StreamType == astits.StreamTypePrivateData {
					klvPID = es.ElementaryPID
					foundKLVPID = true
					break
				}
			}
		}

		// Check if this packet contains KLV data
		if foundKLVPID && data.PES != nil && data.PID == klvPID && !foundKLVData {
			foundKLVData = true
			receivedKLVData = data.PES.Data
			// Verify the KLV data matches what we wrote
			require.Equal(t, []byte{0x00, 0x01, 0x02, 0x03}, receivedKLVData,
				"KLV data content mismatch")
			break
		}
	}

	require.True(t, foundKLVPID, "KLV PID was not found in PMT")
	require.True(t, foundKLVData, "KLV data was not found in the segment")
}

func TestMuxerKLVOnlyTrackRejected(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{klvTrack},
	}

	err := m.Start()
	require.Error(t, err)
	require.Contains(t, err.Error(), "KLV tracks require at least one video or audio track")
}

func TestMuxerKLVFirstTrackWithAudio(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	audioTrack := &Track{
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

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{klvTrack, audioTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	// Verify that the audio track is the leading track, not KLV
	require.False(t, m.mtracksByTrack[klvTrack].isLeading, "KLV track should not be leading")
	require.True(t, m.mtracksByTrack[audioTrack].isLeading, "Audio track should be leading")
}

// TestMuxerKLVSynchronous verifies that a synchronous KLV track (Synchronous: true)
// is registered in the PMT as StreamTypeMetadata (0x15) rather than StreamTypePrivateData
// (0x06), and that PES packets for it carry a PTS.
func TestMuxerKLVSynchronous(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{Synchronous: true},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack, klvTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := range 4 {
		d := time.Duration(i) * time.Second
		pts := int64(d) * 90000 / int64(time.Second)

		err = m.WriteKLV(klvTrack, testTime.Add(d), pts, []byte{0xAA, 0xBB})
		require.NoError(t, err)

		err = m.WriteH264(testVideoTrack, testTime.Add(d), pts, [][]byte{
			testH264SPS,
			{8},
			{5},
		})
		require.NoError(t, err)
	}

	byts, _, err := doRequest(m, "main_stream.m3u8")
	require.NoError(t, err)

	re := regexp.MustCompile(`(.*?_main_seg0\.ts)`)
	ma := re.FindStringSubmatch(string(byts))
	require.NotNil(t, ma, "no segment found in playlist")

	segmentData, _, err := doRequest(m, ma[1])
	require.NoError(t, err)
	require.NotEmpty(t, segmentData)

	demuxer := astits.NewDemuxer(context.Background(), bytes.NewReader(segmentData))

	foundSyncKLV := false
	for {
		data, nextErr := demuxer.NextData()
		if nextErr != nil {
			break
		}
		if data.PMT != nil {
			for _, es := range data.PMT.ElementaryStreams {
				// Synchronous KLV is registered as StreamTypeMetadata (0x15)
				if es.StreamType == astits.StreamTypeMetadata {
					foundSyncKLV = true
				}
			}
		}
	}
	require.True(t, foundSyncKLV, "synchronous KLV PID with StreamTypeMetadata not found in PMT")
}

// TestMuxerKLVMultipleTracks verifies that two KLV tracks (one async, one sync)
// each receive their own distinct PID in the PMT.
func TestMuxerKLVMultipleTracks(t *testing.T) {
	klvAsync := &Track{
		Codec:     &codecs.KLV{Synchronous: false},
		ClockRate: 90000,
	}
	klvSync := &Track{
		Codec:     &codecs.KLV{Synchronous: true},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack, klvAsync, klvSync},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := range 4 {
		d := time.Duration(i) * time.Second
		pts := int64(d) * 90000 / int64(time.Second)

		err = m.WriteKLV(klvAsync, testTime.Add(d), pts, []byte{0x01, 0x02})
		require.NoError(t, err)

		err = m.WriteKLV(klvSync, testTime.Add(d), pts, []byte{0x03, 0x04})
		require.NoError(t, err)

		err = m.WriteH264(testVideoTrack, testTime.Add(d), pts, [][]byte{
			testH264SPS,
			{8},
			{5},
		})
		require.NoError(t, err)
	}

	byts, _, err := doRequest(m, "main_stream.m3u8")
	require.NoError(t, err)

	re := regexp.MustCompile(`(.*?_main_seg0\.ts)`)
	ma := re.FindStringSubmatch(string(byts))
	require.NotNil(t, ma)

	segmentData, _, err := doRequest(m, ma[1])
	require.NoError(t, err)

	demuxer := astits.NewDemuxer(context.Background(), bytes.NewReader(segmentData))

	asyncPIDs := 0
	syncPIDs := 0
	for {
		data, nextErr := demuxer.NextData()
		if nextErr != nil {
			break
		}
		if data.PMT != nil {
			for _, es := range data.PMT.ElementaryStreams {
				switch es.StreamType {
				case astits.StreamTypePrivateData:
					asyncPIDs++
				case astits.StreamTypeMetadata:
					syncPIDs++
				}
			}
		}
	}
	require.Equal(t, 1, asyncPIDs, "expected exactly one async KLV PID (StreamTypePrivateData)")
	require.Equal(t, 1, syncPIDs, "expected exactly one sync KLV PID (StreamTypeMetadata)")
}

// TestMuxerKLVBeforeFirstSegment verifies that KLV data written before the leading
// track has created the first segment is silently dropped without returning an error.
func TestMuxerKLVBeforeFirstSegment(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack, klvTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	// Write KLV before any H264 — no segment exists yet, must be silently dropped.
	err = m.WriteKLV(klvTrack, testTime, 0, []byte{0xDE, 0xAD, 0xBE, 0xEF})
	require.NoError(t, err, "WriteKLV before first segment should not return an error")

	// Now drive segment creation with H264 frames.
	for i := range 4 {
		d := time.Duration(i) * time.Second
		pts := int64(d) * 90000 / int64(time.Second)
		err = m.WriteH264(testVideoTrack, testTime.Add(d), pts, [][]byte{
			testH264SPS,
			{8},
			{5},
		})
		require.NoError(t, err)
	}

	byts, _, err := doRequest(m, "main_stream.m3u8")
	require.NoError(t, err)
	require.Contains(t, string(byts), "seg0.ts")
}

// TestMuxerKLVWriteOnFMP4Variant verifies that calling WriteKLV on a muxer that
// is using the FMP4 variant returns an appropriate error.
func TestMuxerKLVWriteOnFMP4Variant(t *testing.T) {
	m := &Muxer{
		Variant:            MuxerVariantFMP4,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	// Use a KLV track that is not registered in the muxer.
	fakeKLVTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	err = m.WriteKLV(fakeKLVTrack, testTime, 0, []byte{0x01})
	require.Error(t, err)
	require.Contains(t, err.Error(), "MPEG-TS")
}

// TestMuxerKLVFMP4TrackRejectedAtStart verifies that attempting to Start() a
// non-MPEG-TS muxer with a KLV track returns an error immediately.
func TestMuxerKLVFMP4TrackRejectedAtStart(t *testing.T) {
	cases := []struct {
		name     string
		variant  MuxerVariant
		segCount int
	}{
		{"fmp4", MuxerVariantFMP4, 3},
		{"lowLatency", MuxerVariantLowLatency, 7},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			klvTrack := &Track{
				Codec:     &codecs.KLV{},
				ClockRate: 90000,
			}

			m := &Muxer{
				Variant:            tc.variant,
				SegmentCount:       tc.segCount,
				SegmentMinDuration: 1 * time.Second,
				Tracks:             []*Track{testVideoTrack, klvTrack},
			}

			err := m.Start()
			require.Error(t, err)
			require.Contains(t, err.Error(), "MPEG-TS")
		})
	}
}

// TestMuxerKLVEmptyData verifies that writing a zero-length KLV payload does not
// panic and is handled gracefully.
func TestMuxerKLVEmptyData(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack, klvTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := range 4 {
		d := time.Duration(i) * time.Second
		pts := int64(d) * 90000 / int64(time.Second)

		// Write empty KLV payload — must not panic.
		err = m.WriteKLV(klvTrack, testTime.Add(d), pts, []byte{})
		require.NoError(t, err)

		err = m.WriteH264(testVideoTrack, testTime.Add(d), pts, [][]byte{
			testH264SPS,
			{8},
			{5},
		})
		require.NoError(t, err)
	}

	byts, _, err := doRequest(m, "main_stream.m3u8")
	require.NoError(t, err)
	require.Contains(t, string(byts), "seg0.ts")
}

// TestMuxerKLVMultivariantCODECS verifies that the CODECS attribute in the
// multivariant playlist does not contain an empty string for KLV tracks, which
// have no standard HLS codec identifier.
func TestMuxerKLVMultivariantCODECS(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack, klvTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := range 4 {
		d := time.Duration(i) * time.Second
		pts := int64(d) * 90000 / int64(time.Second)

		err = m.WriteKLV(klvTrack, testTime.Add(d), pts, []byte{0x01, 0x02})
		require.NoError(t, err)

		err = m.WriteH264(testVideoTrack, testTime.Add(d), pts, [][]byte{
			testH264SPS,
			{8},
			{5},
		})
		require.NoError(t, err)
	}

	// Fetch the multivariant playlist.
	byts, _, err := doRequest(m, "index.m3u8")
	require.NoError(t, err)

	playlist := string(byts)

	// The CODECS attribute must not contain an empty codec string (e.g. "avc1.64001f,").
	require.NotContains(t, playlist, `CODECS="",`,
		"CODECS attribute must not start with an empty string")
	require.NotContains(t, playlist, `,""`,
		"CODECS attribute must not contain a trailing empty string")

	// Verify the H264 codec IS present.
	require.True(t, strings.Contains(playlist, "avc1."),
		"CODECS attribute must contain the H264 codec string")

	// Verify no bare comma (which signals an empty element) appears inside the CODECS value.
	re := regexp.MustCompile(`CODECS="([^"]*)"`)
	ma := re.FindStringSubmatch(playlist)
	require.NotNil(t, ma, "CODECS attribute not found in multivariant playlist")
	for part := range strings.SplitSeq(ma[1], ",") {
		require.NotEmpty(t, strings.TrimSpace(part),
			"CODECS attribute contains an empty entry: %q", ma[1])
	}
}

// TestMuxerKLVExceedsSegmentMaxSize verifies that writing KLV data that would push
// the segment past SegmentMaxSize returns an error.
func TestMuxerKLVExceedsSegmentMaxSize(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	// testH264SPS(25B) + PPS(1B) + IDR(1B) = 27 bytes for the first H264 write.
	// A SegmentMaxSize of 50 bytes accepts H264 but not an additional 30-byte KLV payload.
	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		SegmentMaxSize:     50,
		Tracks:             []*Track{testVideoTrack, klvTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	// Write the first H264 IDR to create the segment (consumes 27 bytes of the 50-byte limit).
	err = m.WriteH264(testVideoTrack, testTime, 0, [][]byte{testH264SPS, {8}, {5}})
	require.NoError(t, err)

	// Write 30 bytes of KLV into the same segment — 27 + 30 = 57 > 50, must fail.
	err = m.WriteKLV(klvTrack, testTime, 0, make([]byte, 30))
	require.Error(t, err)
	require.Contains(t, err.Error(), "maximum segment size")
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
			testH264SPS,
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
		testH264SPS,
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
				testH264SPS,
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
		testH264SPS,
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
	require.Equal(t, testH264SPS, init1.Tracks[0].Codec.(*mp4codecs.H264).SPS)

	// SPS (720p)
	testH264SPS2 := []byte{
		0x67, 0x64, 0x00, 0x1f, 0xac, 0xd9, 0x40, 0x50,
		0x05, 0xbb, 0x01, 0x6c, 0x80, 0x00, 0x00, 0x03,
		0x00, 0x80, 0x00, 0x00, 0x1e, 0x07, 0x8c, 0x18,
		0xcb,
	}

	err = m.WriteH264(testVideoTrack, testTime, 3*90000, [][]byte{
		testH264SPS2,
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

				err = m.WriteH264(track, testTime, 0, [][]byte{testH264SPS, inStreamPPS, {5}})
				require.NoError(t, err)

				err = m.WriteH264(track, testTime, 2*90000, [][]byte{{5}})
				require.NoError(t, err)

				err = m.WriteH264(track, testTime, 4*90000, [][]byte{{5}})
				require.NoError(t, err)

				h264Codec := getInit(t, m).Tracks[0].Codec.(*mp4codecs.H264)
				require.Equal(t, testH264SPS, h264Codec.SPS)
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
		testH264SPS, // SPS
		{8},         // PPS
		{5},         // IDR
	})
	require.NoError(t, err)

	err = m.WriteH264(testVideoTrack, time.Now(), 1, [][]byte{
		testH264SPS, // SPS
		{8},         // PPS
		{5},         // IDR
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
			testH264SPS,
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
		testH264SPS,
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
						testH264SPS, // SPS
						{8},         // PPS
						{5},         // IDR
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
				testH264SPS, // SPS
				{8},         // PPS
				{5},         // IDR
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
				testH264SPS, // SPS
				{8},         // PPS
				{5},         // IDR
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
