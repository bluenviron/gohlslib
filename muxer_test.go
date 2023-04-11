package gohlslib

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
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

type fakeResponseWriter struct {
	bytes.Buffer
	h          http.Header
	statusCode int
}

func (w *fakeResponseWriter) Header() http.Header {
	return w.h
}

func (w *fakeResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func readPath(m *Muxer, path, msn, part, skip string) ([]byte, error) {
	w := &fakeResponseWriter{
		h: make(http.Header),
	}

	v := url.Values{}
	v.Set("_HLS_msn", msn)
	v.Set("_HLS_part", part)
	v.Set("_HLS_skip", skip)

	r := &http.Request{
		URL: &url.URL{
			Path:     path,
			RawQuery: v.Encode(),
		},
	}

	m.Handle(w, r)

	if w.statusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %v", w.statusCode)
	}

	return w.Bytes(), nil
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
				SegmentDuration: 1 * time.Second,
				VideoTrack:      testVideoTrack,
				AudioTrack:      testAudioTrack,
			}

			err := m.Start()
			require.NoError(t, err)
			defer m.Close()

			// access unit without IDR
			d := 1 * time.Second
			err = m.WriteH26x(testTime.Add(d-1*time.Second), d, [][]byte{
				{0x06},
				{0x07},
			})
			require.NoError(t, err)

			// access unit with IDR
			d = 2 * time.Second
			err = m.WriteH26x(testTime.Add(d-1*time.Second), d, [][]byte{
				testSPS, // SPS
				{8},     // PPS
				{5},     // IDR
			})
			require.NoError(t, err)

			d = 3 * time.Second
			err = m.WriteAudio(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			d = 3500 * time.Millisecond
			err = m.WriteAudio(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			// access unit without IDR
			d = 4 * time.Second
			err = m.WriteH26x(testTime.Add(d-1*time.Second), d, [][]byte{
				{1}, // non-IDR
			})
			require.NoError(t, err)

			d = 4500 * time.Millisecond
			err = m.WriteAudio(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			// access unit with IDR
			d = 6 * time.Second
			err = m.WriteH26x(testTime.Add(d-1*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			// access unit with IDR
			d = 7 * time.Second
			err = m.WriteH26x(testTime.Add(d-1*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			byts, err := readPath(m, "/index.m3u8", "", "", "")
			require.NoError(t, err)

			switch ca {
			case "mpegts":
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:3\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=4512,AVERAGE-BANDWIDTH=3008,"+
					"CODECS=\"avc1.42c028,mp4a.40.2\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
					"stream.m3u8\n", string(byts))

			case "fmp4":
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:9\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=936,AVERAGE-BANDWIDTH=584,"+
					"CODECS=\"avc1.42c028,mp4a.40.2\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
					"stream.m3u8\n", string(byts))

			case "lowLatency":
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:9\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=936,AVERAGE-BANDWIDTH=737,"+
					"CODECS=\"avc1.42c028,mp4a.40.2\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
					"stream.m3u8\n", string(byts))
			}

			byts, err = readPath(m, "stream.m3u8", "", "", "")
			require.NoError(t, err)

			switch ca {
			case "mpegts":
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:3\n` +
					`#EXT-X-ALLOW-CACHE:NO\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4.00000,\n` +
					`(seg0\.ts)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(seg1\.ts)\n$`)
				ma := re.FindStringSubmatch(string(byts))
				require.NotEqual(t, 0, len(ma))

				_, err := readPath(m, ma[2], "", "", "")
				require.NoError(t, err)

			case "fmp4":
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:9\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-MAP:URI="init.mp4"\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4.00000,\n` +
					`(seg0\.mp4)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(seg1\.mp4)\n$`)
				ma := re.FindStringSubmatch(string(byts))
				require.NotEqual(t, 0, len(ma))

				_, err := readPath(m, "init.mp4", "", "", "")
				require.NoError(t, err)

				_, err = readPath(m, ma[2], "", "", "")
				require.NoError(t, err)

			case "lowLatency":
				require.Equal(t,
					"#EXTM3U\n"+
						"#EXT-X-VERSION:9\n"+
						"#EXT-X-TARGETDURATION:4\n"+
						"#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=5.00000,CAN-SKIP-UNTIL=24\n"+
						"#EXT-X-PART-INF:PART-TARGET=2\n"+
						"#EXT-X-MEDIA-SEQUENCE:2\n"+
						"#EXT-X-MAP:URI=\"init.mp4\"\n"+
						"#EXT-X-GAP\n"+
						"#EXTINF:4.00000,\n"+
						"gap.mp4\n"+
						"#EXT-X-GAP\n"+
						"#EXTINF:4.00000,\n"+
						"gap.mp4\n"+
						"#EXT-X-GAP\n"+
						"#EXTINF:4.00000,\n"+
						"gap.mp4\n"+
						"#EXT-X-GAP\n"+
						"#EXTINF:4.00000,\n"+
						"gap.mp4\n"+
						"#EXT-X-GAP\n"+
						"#EXTINF:4.00000,\n"+
						"gap.mp4\n"+
						"#EXT-X-PROGRAM-DATE-TIME:2010-01-01T01:01:02Z\n"+
						"#EXT-X-PART:DURATION=2.00000,URI=\"part0.mp4\",INDEPENDENT=YES\n"+
						"#EXT-X-PART:DURATION=2.00000,URI=\"part1.mp4\"\n"+
						"#EXTINF:4.00000,\n"+
						"seg7.mp4\n"+
						"#EXT-X-PROGRAM-DATE-TIME:2010-01-01T01:01:06Z\n"+
						"#EXT-X-PART:DURATION=1.00000,URI=\"part3.mp4\",INDEPENDENT=YES\n"+
						"#EXTINF:1.00000,\n"+
						"seg8.mp4\n"+
						"#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part4.mp4\"\n", string(byts))

				_, err := readPath(m, "part3.mp4", "", "", "")
				require.NoError(t, err)

				recv := make(chan struct{})

				go func() {
					_, err := readPath(m, "part4.mp4", "", "", "")
					require.NoError(t, err)
					close(recv)
				}()

				d = 9 * time.Second
				err = m.WriteH26x(testTime.Add(d-1*time.Second), d, [][]byte{
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
				Variant:         v,
				SegmentCount:    3,
				SegmentDuration: 1 * time.Second,
				VideoTrack:      testVideoTrack,
			}

			err := m.Start()
			require.NoError(t, err)
			defer m.Close()

			// access unit with IDR
			d := 2 * time.Second
			err = m.WriteH26x(testTime.Add(d-2*time.Second), d, [][]byte{
				testSPS, // SPS
				{8},     // PPS
				{5},     // IDR
			})
			require.NoError(t, err)

			// access unit with IDR
			d = 6 * time.Second
			err = m.WriteH26x(testTime.Add(d-2*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			// access unit with IDR
			d = 7 * time.Second
			err = m.WriteH26x(testTime.Add(d-2*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			byts, err := readPath(m, "index.m3u8", "", "", "")
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
					"#EXT-X-STREAM-INF:BANDWIDTH=936,AVERAGE-BANDWIDTH=428,"+
					"CODECS=\"avc1.42c028\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
					"stream.m3u8\n", string(byts))
			}

			byts, err = readPath(m, "stream.m3u8", "", "", "")
			require.NoError(t, err)

			var ma []string
			if ca == "mpegts" {
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:3\n` +
					`#EXT-X-ALLOW-CACHE:NO\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4.00000,\n` +
					`(seg0\.ts)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(seg1\.ts)\n$`)
				ma = re.FindStringSubmatch(string(byts))
			} else {
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:9\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-MAP:URI="init.mp4"\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4.00000,\n` +
					`(seg0\.mp4)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(seg1\.mp4)\n$`)
				ma = re.FindStringSubmatch(string(byts))
			}
			require.NotEqual(t, 0, len(ma))

			if ca == "mpegts" {
				_, err := readPath(m, ma[2], "", "", "")
				require.NoError(t, err)
			} else {
				_, err := readPath(m, "init.mp4", "", "", "")
				require.NoError(t, err)

				_, err = readPath(m, ma[2], "", "", "")
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
				Variant:         v,
				SegmentCount:    3,
				SegmentDuration: 1 * time.Second,
				AudioTrack:      testAudioTrack,
			}

			err := m.Start()
			require.NoError(t, err)
			defer m.Close()

			for i := 0; i < 100; i++ {
				d := 1 * time.Second
				err = m.WriteAudio(testTime.Add(d-1*time.Second), d, []byte{
					0x01, 0x02, 0x03, 0x04,
				})
				require.NoError(t, err)
			}

			d := 2 * time.Second
			err = m.WriteAudio(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			d = 3 * time.Second
			err = m.WriteAudio(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			byts, err := readPath(m, "index.m3u8", "", "", "")
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
					"#EXT-X-STREAM-INF:BANDWIDTH=37209,AVERAGE-BANDWIDTH=4789,CODECS=\"mp4a.40.2\"\n"+
					"stream.m3u8\n", string(byts))
			}

			byts, err = readPath(m, "stream.m3u8", "", "", "")
			require.NoError(t, err)

			var ma []string
			if ca == "mpegts" {
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:3\n` +
					`#EXT-X-ALLOW-CACHE:NO\n` +
					`#EXT-X-TARGETDURATION:1\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(seg0\.ts)\n$`)
				ma = re.FindStringSubmatch(string(byts))
			} else {
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:9\n` +
					`#EXT-X-TARGETDURATION:2\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-MAP:URI="init.mp4"\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:2.32200,\n` +
					`(seg0\.mp4)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:0.02322,\n` +
					`(seg1\.mp4)\n$`)
				ma = re.FindStringSubmatch(string(byts))
			}
			require.NotEqual(t, 0, len(ma))

			if ca == "mpegts" {
				_, err := readPath(m, ma[2], "", "", "")
				require.NoError(t, err)
			} else {
				_, err := readPath(m, "init.mp4", "", "", "")
				require.NoError(t, err)

				_, err = readPath(m, ma[2], "", "", "")
				require.NoError(t, err)
			}
		})
	}
}

func TestMuxerCloseBeforeFirstSegmentReader(t *testing.T) {
	m := &Muxer{
		Variant:         MuxerVariantMPEGTS,
		SegmentCount:    3,
		SegmentDuration: 1 * time.Second,
		VideoTrack:      testVideoTrack,
	}

	err := m.Start()
	require.NoError(t, err)

	// access unit with IDR
	err = m.WriteH26x(testTime, 2*time.Second, [][]byte{
		testSPS, // SPS
		{8},     // PPS
		{5},     // IDR
	})
	require.NoError(t, err)

	m.Close()

	b, _ := readPath(m, "stream.m3u8", "", "", "")
	require.Equal(t, []byte(nil), b)
}

func TestMuxerMaxSegmentSize(t *testing.T) {
	m := &Muxer{
		Variant:         MuxerVariantMPEGTS,
		SegmentCount:    3,
		SegmentDuration: 1 * time.Second,
		SegmentMaxSize:  1,
		VideoTrack:      testVideoTrack,
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH26x(testTime, 2*time.Second, [][]byte{
		testSPS,
		{5}, // IDR
	})
	require.EqualError(t, err, "reached maximum segment size")
}

func TestMuxerDoubleRead(t *testing.T) {
	m := &Muxer{
		Variant:         MuxerVariantMPEGTS,
		SegmentCount:    3,
		SegmentDuration: 1 * time.Second,
		VideoTrack:      testVideoTrack,
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH26x(testTime, 0, [][]byte{
		testSPS,
		{5}, // IDR
		{1},
	})
	require.NoError(t, err)

	err = m.WriteH26x(testTime, 2*time.Second, [][]byte{
		{5}, // IDR
		{2},
	})
	require.NoError(t, err)

	byts, err := readPath(m, "stream.m3u8", "", "", "")
	require.NoError(t, err)

	re := regexp.MustCompile(`^#EXTM3U\n` +
		`#EXT-X-VERSION:3\n` +
		`#EXT-X-ALLOW-CACHE:NO\n` +
		`#EXT-X-TARGETDURATION:2\n` +
		`#EXT-X-MEDIA-SEQUENCE:0\n` +
		`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
		`#EXTINF:2.00000,\n` +
		`(seg0\.ts)\n$`)
	ma := re.FindStringSubmatch(string(byts))
	require.NotEqual(t, 0, len(ma))

	byts1, err := readPath(m, ma[2], "", "", "")
	require.NoError(t, err)

	byts2, err := readPath(m, ma[2], "", "", "")
	require.NoError(t, err)
	require.Equal(t, byts1, byts2)
}

func TestMuxerFMP4ZeroDuration(t *testing.T) {
	m := &Muxer{
		Variant:         MuxerVariantFMP4,
		SegmentCount:    3,
		SegmentDuration: 1 * time.Second,
		VideoTrack:      testVideoTrack,
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH26x(time.Now(), 0, [][]byte{
		testSPS, // SPS
		{8},     // PPS
		{5},     // IDR
	})
	require.NoError(t, err)

	err = m.WriteH26x(time.Now(), 1*time.Nanosecond, [][]byte{
		testSPS, // SPS
		{8},     // PPS
		{5},     // IDR
	})
	require.NoError(t, err)
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
				Variant:         v,
				SegmentCount:    3,
				SegmentDuration: 1 * time.Second,
				VideoTrack:      testVideoTrack,
				Directory:       dir,
			}

			err = m.Start()
			require.NoError(t, err)

			err = m.WriteH26x(testTime, 0, [][]byte{
				testSPS,
				{5}, // IDR
				{1},
			})
			require.NoError(t, err)

			err = m.WriteH26x(testTime, 2*time.Second, [][]byte{
				{5}, // IDR
				{2},
			})
			require.NoError(t, err)

			var ext string
			if ca == "mpegts" {
				ext = "ts"
			} else {
				ext = "mp4"
			}

			_, err = os.ReadFile(filepath.Join(dir, "seg0."+ext))
			require.NoError(t, err)

			m.Close()

			_, err = os.ReadFile(filepath.Join(dir, "seg0."+ext))
			require.Error(t, err)
		})
	}
}
