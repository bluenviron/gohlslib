package gohlslib

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
	"github.com/stretchr/testify/require"
)

func TestClientKLVMPEGTS(t *testing.T) {
	httpServ := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/stream.m3u8":
				w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
				w.Write([]byte("#EXTM3U\n" +
					"#EXT-X-VERSION:3\n" +
					"#EXT-X-ALLOW-CACHE:NO\n" +
					"#EXT-X-TARGETDURATION:2\n" +
					"#EXT-X-MEDIA-SEQUENCE:0\n" +
					"#EXTINF:2,\n" +
					"segment1.ts\n" +
					"#EXTINF:2,\n" +
					"segment1.ts\n" +
					"#EXTINF:2,\n" +
					"segment1.ts\n"))

			case r.Method == http.MethodGet && r.URL.Path == "/segment1.ts":
				w.Header().Set("Content-Type", `video/MP2T`)

				h264Track := &mpegts.Track{
					Codec: &tscodecs.H264{},
				}
				klvTrack := &mpegts.Track{
					Codec: &tscodecs.KLV{},
				}
				mw := &mpegts.Writer{W: w, Tracks: []*mpegts.Track{h264Track, klvTrack}}
				err := mw.Initialize()
				require.NoError(t, err)

				err = mw.WriteH264(
					h264Track,
					90000,
					90000,
					[][]byte{
						{7, 1, 2, 3}, // SPS
						{8},          // PPS
						{5},          // IDR
					},
				)
				require.NoError(t, err)

				err = mw.WriteKLV(
					klvTrack,
					90000,
					[]byte{0x06, 0x0e, 0x2b, 0x34},
				)
				require.NoError(t, err)
			}
		}),
	}

	ln, err := net.Listen("tcp", "localhost:5780")
	require.NoError(t, err)

	go httpServ.Serve(ln)
	defer httpServ.Shutdown(context.Background())

	recv := make(chan struct{})

	var c *Client
	c = &Client{
		URI: "http://localhost:5780/stream.m3u8",
		OnTracks: func(tracks []*Track) error {
			require.Equal(t, 2, len(tracks))
			require.Equal(t, &codecs.H264{}, tracks[0].Codec)
			require.Equal(t, &codecs.KLV{Synchronous: false}, tracks[1].Codec)

			c.OnDataKLV(tracks[1], func(_ int64, data []byte) {
				require.Equal(t, []byte{0x06, 0x0e, 0x2b, 0x34}, data)
				select {
				case <-recv:
				default:
					close(recv)
				}
			})
			return nil
		},
	}

	err = c.Start()
	require.NoError(t, err)
	defer c.Close()

	<-recv
}

func TestClientKLVSynchronousMPEGTS(t *testing.T) {
	httpServ := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/stream.m3u8":
				w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
				w.Write([]byte("#EXTM3U\n" +
					"#EXT-X-VERSION:3\n" +
					"#EXT-X-ALLOW-CACHE:NO\n" +
					"#EXT-X-TARGETDURATION:2\n" +
					"#EXT-X-MEDIA-SEQUENCE:0\n" +
					"#EXTINF:2,\n" +
					"segment1.ts\n" +
					"#EXTINF:2,\n" +
					"segment1.ts\n" +
					"#EXTINF:2,\n" +
					"segment1.ts\n"))

			case r.Method == http.MethodGet && r.URL.Path == "/segment1.ts":
				w.Header().Set("Content-Type", `video/MP2T`)

				h264Track := &mpegts.Track{
					Codec: &tscodecs.H264{},
				}
				klvTrack := &mpegts.Track{
					Codec: &tscodecs.KLV{Synchronous: true},
				}
				mw := &mpegts.Writer{W: w, Tracks: []*mpegts.Track{h264Track, klvTrack}}
				err := mw.Initialize()
				require.NoError(t, err)

				err = mw.WriteH264(
					h264Track,
					90000,
					90000,
					[][]byte{
						{7, 1, 2, 3}, // SPS
						{8},          // PPS
						{5},          // IDR
					},
				)
				require.NoError(t, err)

				err = mw.WriteKLV(
					klvTrack,
					90000,
					[]byte{0x06, 0x0e, 0x2b, 0x34},
				)
				require.NoError(t, err)
			}
		}),
	}

	ln, err := net.Listen("tcp", "localhost:5780")
	require.NoError(t, err)

	go httpServ.Serve(ln)
	defer httpServ.Shutdown(context.Background())

	recv := make(chan struct{})

	var c *Client
	c = &Client{
		URI: "http://localhost:5780/stream.m3u8",
		OnTracks: func(tracks []*Track) error {
			require.Equal(t, 2, len(tracks))
			require.Equal(t, &codecs.KLV{Synchronous: true}, tracks[1].Codec)

			c.OnDataKLV(tracks[1], func(_ int64, data []byte) {
				require.Equal(t, []byte{0x06, 0x0e, 0x2b, 0x34}, data)
				select {
				case <-recv:
				default:
					close(recv)
				}
			})
			return nil
		},
	}

	err = c.Start()
	require.NoError(t, err)
	defer c.Close()

	<-recv
}
