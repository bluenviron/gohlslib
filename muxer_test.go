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

	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/gohlslib/pkg/codecs"
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
}

var testAudioTrack = &Track{
	Codec: &codecs.MPEG4Audio{
		Config: mpeg4audio.Config{
			Type:         2,
			SampleRate:   44100,
			ChannelCount: 2,
		},
	},
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

func TestMuxerVideoAudio(t *testing.T) {
	for _, ca := range []string{
		"mpegts",
		"fmp4",
		"lowLatency",
	} {
		t.Run(ca, func(t *testing.T) {
			var v MuxerVariant
			switch ca {
			case "mpegts":
				v = MuxerVariantMPEGTS

			case "fmp4":
				v = MuxerVariantFMP4

			case "lowLatency":
				v = MuxerVariantLowLatency
			}

			m := &Muxer{
				Variant: v,
				SegmentCount: func() int {
					if ca == "lowLatency" {
						return 7
					}
					return 3
				}(),
				SegmentMinDuration: 1 * time.Second,
				VideoTrack:         testVideoTrack,
				AudioTrack:         testAudioTrack,
			}

			err := m.Start()
			require.NoError(t, err)
			defer m.Close()

			// access unit without IDR
			d := 1 * time.Second
			err = m.WriteH264(testTime.Add(d-1*time.Second), d, [][]byte{
				{1}, // non-IDR
			})
			require.NoError(t, err)

			// access unit with IDR
			d = 2 * time.Second
			err = m.WriteH264(testTime.Add(d-1*time.Second), d, [][]byte{
				testSPS, // SPS
				{8},     // PPS
				{5},     // IDR
			})
			require.NoError(t, err)

			d = 3 * time.Second
			err = m.WriteMPEG4Audio(testTime.Add(d-1*time.Second), d, [][]byte{{
				0x01, 0x02, 0x03, 0x04,
			}})
			require.NoError(t, err)

			d = 3500 * time.Millisecond
			err = m.WriteMPEG4Audio(testTime.Add(d-1*time.Second), d, [][]byte{{
				0x01, 0x02, 0x03, 0x04,
			}})
			require.NoError(t, err)

			// access unit without IDR
			d = 4 * time.Second
			err = m.WriteH264(testTime.Add(d-1*time.Second), d, [][]byte{
				{1}, // non-IDR
			})
			require.NoError(t, err)

			d = 4500 * time.Millisecond
			err = m.WriteMPEG4Audio(testTime.Add(d-1*time.Second), d, [][]byte{{
				0x01, 0x02, 0x03, 0x04,
			}})
			require.NoError(t, err)

			// access unit with IDR
			d = 6 * time.Second
			err = m.WriteH264(testTime.Add(d-1*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			// access unit with IDR
			d = 7 * time.Second
			err = m.WriteH264(testTime.Add(d-1*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			byts, h, err := doRequest(m, "/index.m3u8?key=value")
			require.NoError(t, err)
			require.Equal(t, "application/vnd.apple.mpegurl", h.Get("Content-Type"))
			require.Equal(t, "max-age=30", h.Get("Cache-Control"))

			switch ca {
			case "mpegts":
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:3\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=4512,AVERAGE-BANDWIDTH=3008,"+
					"CODECS=\"avc1.42c028,mp4a.40.2\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
					"stream.m3u8?key=value\n", string(byts))

			case "fmp4":
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:9\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=872,AVERAGE-BANDWIDTH=558,"+
					"CODECS=\"avc1.42c028,mp4a.40.2\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
					"stream.m3u8?key=value\n", string(byts))

			case "lowLatency":
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:9\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=872,AVERAGE-BANDWIDTH=705,"+
					"CODECS=\"avc1.42c028,mp4a.40.2\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
					"stream.m3u8?key=value\n", string(byts))
			}

			byts, h, err = doRequest(m, "stream.m3u8?key=value")
			require.NoError(t, err)
			require.Equal(t, "application/vnd.apple.mpegurl", h.Get("Content-Type"))
			require.Equal(t, "no-cache", h.Get("Cache-Control"))

			switch ca {
			case "mpegts":
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:3\n` +
					`#EXT-X-ALLOW-CACHE:NO\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4.00000,\n` +
					`(.*?_seg0\.ts\?key=value)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(.*?_seg1\.ts\?key=value)\n$`)
				require.Regexp(t, re, string(byts))
				ma := re.FindStringSubmatch(string(byts))

				_, h, err := doRequest(m, ma[2])
				require.NoError(t, err)
				require.Equal(t, "video/MP2T", h.Get("Content-Type"))
				require.Equal(t, "max-age=3600", h.Get("Cache-Control"))

			case "fmp4":
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
				ma := re.FindStringSubmatch(string(byts))

				_, h, err := doRequest(m, ma[1])
				require.NoError(t, err)
				require.Equal(t, "video/mp4", h.Get("Content-Type"))
				require.Equal(t, "max-age=30", h.Get("Cache-Control"))

				_, h, err = doRequest(m, ma[3])
				require.NoError(t, err)
				require.Equal(t, "video/mp4", h.Get("Content-Type"))
				require.Equal(t, "max-age=3600", h.Get("Cache-Control"))

			case "lowLatency":
				re := regexp.MustCompile(
					`^#EXTM3U\n` +
						`#EXT-X-VERSION:10\n` +
						`#EXT-X-TARGETDURATION:4\n` +
						`#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=5.00000,CAN-SKIP-UNTIL=24.00000\n` +
						`#EXT-X-PART-INF:PART-TARGET=2.00000\n` +
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
				ma := re.FindStringSubmatch(string(byts))

				_, h, err := doRequest(m, ma[4])
				require.NoError(t, err)
				require.Equal(t, "video/mp4", h.Get("Content-Type"))
				require.Equal(t, "max-age=3600", h.Get("Cache-Control"))

				recv := make(chan struct{})

				go func() {
					_, _, err2 := doRequest(m, ma[5])
					require.NoError(t, err2)
					close(recv)
				}()

				d = 9 * time.Second
				err = m.WriteH264(testTime.Add(d-1*time.Second), d, [][]byte{
					{1}, // non-IDR
				})
				require.NoError(t, err)

				<-recv
			}
		})
	}
}

func TestMuxerVideoOnly(t *testing.T) {
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
				SegmentCount:       3,
				SegmentMinDuration: 1 * time.Second,
				VideoTrack:         testVideoTrack,
			}

			err := m.Start()
			require.NoError(t, err)
			defer m.Close()

			// access unit with IDR
			d := 2 * time.Second
			err = m.WriteH264(testTime.Add(d-2*time.Second), d, [][]byte{
				testSPS, // SPS
				{8},     // PPS
				{5},     // IDR
			})
			require.NoError(t, err)

			// access unit with IDR
			d = 6 * time.Second
			err = m.WriteH264(testTime.Add(d-2*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			// access unit with IDR
			d = 7 * time.Second
			err = m.WriteH264(testTime.Add(d-2*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			byts, _, err := doRequest(m, "index.m3u8")
			require.NoError(t, err)

			if ca == "mpegts" {
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:3\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=4512,AVERAGE-BANDWIDTH=1804,"+
					"CODECS=\"avc1.42c028\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
					"stream.m3u8\n", string(byts))
			} else {
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:9\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=872,AVERAGE-BANDWIDTH=403,"+
					"CODECS=\"avc1.42c028\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
					"stream.m3u8\n", string(byts))
			}

			byts, _, err = doRequest(m, "stream.m3u8")
			require.NoError(t, err)

			var re *regexp.Regexp
			if ca == "mpegts" {
				re = regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:3\n` +
					`#EXT-X-ALLOW-CACHE:NO\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4.00000,\n` +
					`(.*?_seg0\.ts)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(.*?_seg1\.ts)\n$`)
			} else {
				re = regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:10\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-MAP:URI="(.*?_init.mp4)"\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4.00000,\n` +
					`(.*?_seg0\.mp4)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(.*?_seg1\.mp4)\n$`)
			}
			require.Regexp(t, re, string(byts))
			ma := re.FindStringSubmatch(string(byts))

			if ca == "mpegts" {
				_, _, err := doRequest(m, ma[2])
				require.NoError(t, err)
			} else {
				_, _, err := doRequest(m, ma[1])
				require.NoError(t, err)

				_, _, err = doRequest(m, ma[3])
				require.NoError(t, err)
			}
		})
	}
}

func TestMuxerAudioOnly(t *testing.T) {
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
				SegmentCount:       3,
				SegmentMinDuration: 1 * time.Second,
				AudioTrack:         testAudioTrack,
			}

			err := m.Start()
			require.NoError(t, err)
			defer m.Close()

			for i := 0; i < 100; i++ {
				d := 1 * time.Second
				err = m.WriteMPEG4Audio(testTime.Add(d-1*time.Second), d, [][]byte{{
					0x01, 0x02, 0x03, 0x04,
				}})
				require.NoError(t, err)
			}

			d := 2 * time.Second
			err = m.WriteMPEG4Audio(testTime.Add(d-1*time.Second), d, [][]byte{{
				0x01, 0x02, 0x03, 0x04,
			}})
			require.NoError(t, err)

			d = 3 * time.Second
			err = m.WriteMPEG4Audio(testTime.Add(d-1*time.Second), d, [][]byte{{
				0x01, 0x02, 0x03, 0x04,
			}})
			require.NoError(t, err)

			byts, _, err := doRequest(m, "index.m3u8")
			require.NoError(t, err)

			if ca == "mpegts" {
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:3\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=451200,AVERAGE-BANDWIDTH=451200,CODECS=\"mp4a.40.2\"\n"+
					"stream.m3u8\n", string(byts))
			} else {
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:9\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=10368,AVERAGE-BANDWIDTH=5616,CODECS=\"mp4a.40.2\"\n"+
					"stream.m3u8\n", string(byts))
			}

			byts, _, err = doRequest(m, "stream.m3u8")
			require.NoError(t, err)

			var re *regexp.Regexp
			if ca == "mpegts" {
				re = regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:3\n` +
					`#EXT-X-ALLOW-CACHE:NO\n` +
					`#EXT-X-TARGETDURATION:1\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(.*?_seg0\.ts)\n$`)
			} else {
				re = regexp.MustCompile(`^#EXTM3U\n` +
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
			}
			require.Regexp(t, re, string(byts))
			ma := re.FindStringSubmatch(string(byts))

			if ca == "mpegts" {
				_, _, err := doRequest(m, ma[2])
				require.NoError(t, err)
			} else {
				_, _, err := doRequest(m, ma[1])
				require.NoError(t, err)

				_, _, err = doRequest(m, ma[3])
				require.NoError(t, err)
			}
		})
	}
}

func TestMuxerCloseBeforeData(t *testing.T) {
	m := &Muxer{
		Variant:            MuxerVariantFMP4,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		VideoTrack: &Track{
			Codec: &codecs.AV1{},
		},
	}

	err := m.Start()
	require.NoError(t, err)

	m.Close()

	b, _, _ := doRequest(m, "index.m3u8")
	require.Equal(t, []byte(nil), b)

	b, _, _ = doRequest(m, "stream.m3u8")
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
		VideoTrack:         testVideoTrack,
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(testTime, 2*time.Second, [][]byte{
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
		VideoTrack:         testVideoTrack,
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(testTime, 0, [][]byte{
		testSPS,
		{5}, // IDR
		{1},
	})
	require.NoError(t, err)

	err = m.WriteH264(testTime, 2*time.Second, [][]byte{
		{5}, // IDR
		{2},
	})
	require.NoError(t, err)

	byts, _, err := doRequest(m, "stream.m3u8")
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
				VideoTrack:         testVideoTrack,
				Directory:          dir,
			}

			err = m.Start()
			require.NoError(t, err)

			err = m.WriteH264(testTime, 0, [][]byte{
				testSPS,
				{5}, // IDR
				{1},
			})
			require.NoError(t, err)

			err = m.WriteH264(testTime, 2*time.Second, [][]byte{
				{5}, // IDR
				{2},
			})
			require.NoError(t, err)

			err = m.WriteH264(testTime, 3*time.Second, [][]byte{
				{5}, // IDR
				{2},
			})
			require.NoError(t, err)

			byts, _, err := doRequest(m, "stream.m3u8")
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
		VideoTrack:         testVideoTrack,
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(testTime, 0, [][]byte{
		testSPS,
		{5}, // IDR
		{1},
	})
	require.NoError(t, err)

	err = m.WriteH264(testTime, 1*time.Second, [][]byte{
		{5}, // IDR
		{2},
	})
	require.NoError(t, err)

	err = m.WriteH264(testTime, 2*time.Second, [][]byte{
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
		"stream.m3u8\n", string(bu))

	bu, _, err = doRequest(m, m.prefix+"_init.mp4")
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

	err = m.WriteH264(testTime, 3*time.Second, [][]byte{
		testSPS2,
		{0x65, 0x88, 0x84, 0x00, 0x33, 0xff}, // IDR
		{2},
	})
	require.NoError(t, err)

	err = m.WriteH264(testTime, 5*time.Second, [][]byte{
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
		"stream.m3u8\n", string(bu))

	bu, _, err = doRequest(m, m.prefix+"_init.mp4")
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
		VideoTrack:         testVideoTrack,
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(time.Now(), 0, [][]byte{
		testSPS, // SPS
		{8},     // PPS
		{5},     // IDR
	})
	require.NoError(t, err)

	err = m.WriteH264(time.Now(), 1*time.Nanosecond, [][]byte{
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
		VideoTrack:         testVideoTrack,
		AudioTrack:         testAudioTrack,
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteMPEG4Audio(testTime, -9*time.Second, [][]byte{
		{1, 2, 3, 4},
	})
	require.NoError(t, err)

	// this is skipped
	err = m.WriteH264(testTime, -11*time.Second, [][]byte{
		testSPS,
		{5}, // IDR
		{1},
	})
	require.NoError(t, err)

	err = m.WriteH264(testTime, -9*time.Second, [][]byte{
		testSPS,
		{5}, // IDR
		{1},
	})
	require.NoError(t, err)

	err = m.WriteH264(testTime, -8*time.Second, [][]byte{
		{5}, // IDR
		{2},
	})
	require.NoError(t, err)

	// this is skipped
	err = m.WriteMPEG4Audio(testTime, -11*time.Second, [][]byte{
		{1, 2, 3, 4},
	})
	require.NoError(t, err)

	err = m.WriteMPEG4Audio(testTime, -8*time.Second, [][]byte{
		{5, 6, 7, 8},
	})
	require.NoError(t, err)

	err = m.WriteMPEG4Audio(testTime, -7*time.Second, [][]byte{
		{9, 10, 11, 12},
	})
	require.NoError(t, err)

	// switch segment
	err = m.WriteH264(testTime, -7*time.Second, [][]byte{
		{5}, // IDR
		{3},
	})
	require.NoError(t, err)

	bu, _, err := doRequest(m, m.prefix+"_seg0.mp4")
	require.NoError(t, err)

	var parts fmp4.Parts
	err = parts.Unmarshal(bu)
	require.NoError(t, err)

	require.Equal(t, fmp4.Parts{{
		Tracks: []*fmp4.PartTrack{
			{
				ID:       1,
				BaseTime: 90000,
				Samples: []*fmp4.PartSample{
					{
						Duration: 90000,
						Payload: []byte{
							0x00, 0x00, 0x00, 0x19, 0x67, 0x42, 0xc0, 0x28,
							0xd9, 0x00, 0x78, 0x02, 0x27, 0xe5, 0x84, 0x00,
							0x00, 0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00,
							0xf0, 0x3c, 0x60, 0xc9, 0x20, 0x00, 0x00, 0x00,
							0x01, 0x05, 0x00, 0x00, 0x00, 0x01, 0x01,
						},
					},
					{
						Duration: 90000,
						Payload: []byte{
							0x00, 0x00, 0x00, 0x01, 0x05, 0x00, 0x00, 0x00,
							0x01, 0x02,
						},
					},
				},
			},
			{
				ID:       2,
				BaseTime: 44100,
				Samples: []*fmp4.PartSample{
					{
						Duration: 44100,
						Payload:  []byte{1, 2, 3, 4},
					},
					{
						Duration: 44100,
						Payload:  []byte{5, 6, 7, 8},
					},
				},
			},
		},
	}}, parts)
}

func TestMuxerFMP4SequenceNumber(t *testing.T) {
	m := &Muxer{
		Variant:         MuxerVariantLowLatency,
		SegmentCount:    7,
		SegmentDuration: 2 * time.Second,
		VideoTrack:      testVideoTrack,
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(testTime, 0, [][]byte{
		testSPS,
		{5}, // IDR
		{1},
	})
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		err = m.WriteH264(testTime, (1+time.Duration(i))*time.Second, [][]byte{
			{1}, // non IDR
		})
		require.NoError(t, err)
	}

	for i := 0; i < 3; i++ {
		buf, _, err := doRequest(m, m.prefix+"_part"+strconv.FormatInt(int64(i), 10)+".mp4")
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
				Variant:         v,
				SegmentCount:    7,
				SegmentDuration: 1 * time.Second,
				VideoTrack:      testVideoTrack,
				Directory:       "/nonexisting",
			}

			err := m.Start()
			require.NoError(t, err)
			defer m.Close()

			for i := 0; i < 2; i++ {
				err := m.WriteH264(testTime, time.Duration(i)*time.Second, [][]byte{
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
		Variant:         MuxerVariantLowLatency,
		SegmentCount:    7,
		SegmentDuration: 1 * time.Second,
		VideoTrack:      testVideoTrack,
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := 0; i < 2; i++ {
		err := m.WriteH264(testTime, time.Duration(i)*time.Second, [][]byte{
			testSPS, // SPS
			{8},     // PPS
			{5},     // IDR
		})
		require.NoError(t, err)
	}

	v := url.Values{}
	v.Set("_HLS_msn", "1")
	v.Set("_HLS_part", "0")

	r := &http.Request{
		URL: &url.URL{
			Path:     "stream.m3u8",
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
		Variant:         MuxerVariantLowLatency,
		SegmentCount:    7,
		SegmentDuration: 1 * time.Second,
		VideoTrack:      testVideoTrack,
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := 0; i < 2; i++ {
		err := m.WriteH264(testTime, time.Duration(i)*time.Second, [][]byte{
			testSPS, // SPS
			{8},     // PPS
			{5},     // IDR
		})
		require.NoError(t, err)
	}

	byts, _, err := doRequest(m, "stream.m3u8")
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
		byts, _, err := doRequest(m, ma[1])
		require.NoError(t, err)
		preloadDone <- byts
	}()

	select {
	case <-preloadDone:
		t.Error("should not happen")
	case <-time.After(500 * time.Millisecond):
	}

	err = m.WriteH264(testTime, 3*time.Second, [][]byte{
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
