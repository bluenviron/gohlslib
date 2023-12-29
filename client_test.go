package gohlslib

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/asticode/go-astits"

	"github.com/aler9/writerseeker"
	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
)

var serverCert = []byte(`-----BEGIN CERTIFICATE-----
MIIDazCCAlOgAwIBAgIUXw1hEC3LFpTsllv7D3ARJyEq7sIwDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yMDEyMTMxNzQ0NThaFw0zMDEy
MTExNzQ0NThaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw
HwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQDG8DyyS51810GsGwgWr5rjJK7OE1kTTLSNEEKax8Bj
zOyiaz8rA2JGl2VUEpi2UjDr9Cm7nd+YIEVs91IIBOb7LGqObBh1kGF3u5aZxLkv
NJE+HrLVvUhaDobK2NU+Wibqc/EI3DfUkt1rSINvv9flwTFu1qHeuLWhoySzDKEp
OzYxpFhwjVSokZIjT4Red3OtFz7gl2E6OAWe2qoh5CwLYVdMWtKR0Xuw3BkDPk9I
qkQKx3fqv97LPEzhyZYjDT5WvGrgZ1WDAN3booxXF3oA1H3GHQc4m/vcLatOtb8e
nI59gMQLEbnp08cl873bAuNuM95EZieXTHNbwUnq5iybAgMBAAGjUzBRMB0GA1Ud
DgQWBBQBKhJh8eWu0a4au9X/2fKhkFX2vjAfBgNVHSMEGDAWgBQBKhJh8eWu0a4a
u9X/2fKhkFX2vjAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQBj
3aCW0YPKukYgVK9cwN0IbVy/D0C1UPT4nupJcy/E0iC7MXPZ9D/SZxYQoAkdptdO
xfI+RXkpQZLdODNx9uvV+cHyZHZyjtE5ENu/i5Rer2cWI/mSLZm5lUQyx+0KZ2Yu
tEI1bsebDK30msa8QSTn0WidW9XhFnl3gRi4wRdimcQapOWYVs7ih+nAlSvng7NI
XpAyRs8PIEbpDDBMWnldrX4TP6EWYUi49gCp8OUDRREKX3l6Ls1vZ02F34yHIt/7
7IV/XSKG096bhW+icKBWV0IpcEsgTzPK1J1hMxgjhzIMxGboAeUU+kidthOob6Sd
XQxaORfgM//NzX9LhUPk
-----END CERTIFICATE-----
`)

var serverKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEAxvA8skudfNdBrBsIFq+a4ySuzhNZE0y0jRBCmsfAY8zsoms/
KwNiRpdlVBKYtlIw6/Qpu53fmCBFbPdSCATm+yxqjmwYdZBhd7uWmcS5LzSRPh6y
1b1IWg6GytjVPlom6nPxCNw31JLda0iDb7/X5cExbtah3ri1oaMkswyhKTs2MaRY
cI1UqJGSI0+EXndzrRc+4JdhOjgFntqqIeQsC2FXTFrSkdF7sNwZAz5PSKpECsd3
6r/eyzxM4cmWIw0+Vrxq4GdVgwDd26KMVxd6ANR9xh0HOJv73C2rTrW/HpyOfYDE
CxG56dPHJfO92wLjbjPeRGYnl0xzW8FJ6uYsmwIDAQABAoIBACi0BKcyQ3HElSJC
kaAao+Uvnzh4yvPg8Nwf5JDIp/uDdTMyIEWLtrLczRWrjGVZYbsVROinP5VfnPTT
kYwkfKINj2u+gC6lsNuPnRuvHXikF8eO/mYvCTur1zZvsQnF5kp4GGwIqr+qoPUP
bB0UMndG1PdpoMryHe+JcrvTrLHDmCeH10TqOwMsQMLHYLkowvxwJWsmTY7/Qr5S
Wm3PPpOcW2i0uyPVuyuv4yD1368fqnqJ8QFsQp1K6QtYsNnJ71Hut1/IoxK/e6hj
5Z+byKtHVtmcLnABuoOT7BhleJNFBksX9sh83jid4tMBgci+zXNeGmgqo2EmaWAb
agQslkECgYEA8B1rzjOHVQx/vwSzDa4XOrpoHQRfyElrGNz9JVBvnoC7AorezBXQ
M9WTHQIFTGMjzD8pb+YJGi3gj93VN51r0SmJRxBaBRh1ZZI9kFiFzngYev8POgD3
ygmlS3kTHCNxCK/CJkB+/jMBgtPj5ygDpCWVcTSuWlQFphePkW7jaaECgYEA1Blz
ulqgAyJHZaqgcbcCsI2q6m527hVr9pjzNjIVmkwu38yS9RTCgdlbEVVDnS0hoifl
+jVMEGXjF3xjyMvL50BKbQUH+KAa+V4n1WGlnZOxX9TMny8MBjEuSX2+362vQ3BX
4vOlX00gvoc+sY+lrzvfx/OdPCHQGVYzoKCxhLsCgYA07HcviuIAV/HsO2/vyvhp
xF5gTu+BqNUHNOZDDDid+ge+Jre2yfQLCL8VPLXIQW3Jff53IH/PGl+NtjphuLvj
7UDJvgvpZZuymIojP6+2c3gJ3CASC9aR3JBnUzdoE1O9s2eaoMqc4scpe+SWtZYf
3vzSZ+cqF6zrD/Rf/M35IQKBgHTU4E6ShPm09CcoaeC5sp2WK8OevZw/6IyZi78a
r5Oiy18zzO97U/k6xVMy6F+38ILl/2Rn31JZDVJujniY6eSkIVsUHmPxrWoXV1HO
y++U32uuSFiXDcSLarfIsE992MEJLSAynbF1Rsgsr3gXbGiuToJRyxbIeVy7gwzD
94TpAoGAY4/PejWQj9psZfAhyk5dRGra++gYRQ/gK1IIc1g+Dd2/BxbT/RHr05GK
6vwrfjsoRyMWteC1SsNs/CurjfQ/jqCfHNP5XPvxgd5Ec8sRJIiV7V5RTuWJsPu1
+3K6cnKEyg+0ekYmLertRFIY6SwWmY1fyKgTvxudMcsBY7dC4xs=
-----END RSA PRIVATE KEY-----
`)

func writeTempFile(byts []byte) (string, error) {
	tmpf, err := os.CreateTemp(os.TempDir(), "rtsp-")
	if err != nil {
		return "", err
	}
	defer tmpf.Close()

	_, err = tmpf.Write(byts)
	if err != nil {
		return "", err
	}

	return tmpf.Name(), nil
}

func mpegtsSegment(t *testing.T, w io.Writer) {
	mux := astits.NewMuxer(context.Background(), w)

	err := mux.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: 256,
		StreamType:    astits.StreamTypeH264Video,
	})
	require.NoError(t, err)

	mux.SetPCRPID(256)

	_, err = mux.WriteTables()
	require.NoError(t, err)

	enc, _ := h264.AnnexBMarshal([][]byte{
		{7, 1, 2, 3}, // SPS
		{8},          // PPS
		{5},          // IDR
	})

	_, err = mux.WriteData(&astits.MuxerData{
		PID: 256,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: &astits.PESOptionalHeader{
					MarkerBits:      2,
					PTSDTSIndicator: astits.PTSDTSIndicatorBothPresent,
					PTS:             &astits.ClockReference{Base: 90000},                   // +1 sec
					DTS:             &astits.ClockReference{Base: 0x1FFFFFFFF - 90000 + 1}, // -1 sec
				},
				StreamID: 224, // = video
			},
			Data: enc,
		},
	})
	require.NoError(t, err)
}

type marshaler interface {
	Marshal(w io.WriteSeeker) error
}

func mp4ToWriter(i marshaler, w io.Writer) error {
	ws := &writerseeker.WriterSeeker{}
	err := i.Marshal(ws)
	if err != nil {
		return err
	}

	_, err = w.Write(ws.Bytes())
	return err
}

func TestClientMPEGTS(t *testing.T) {
	for _, ca := range []string{
		"plain",
		"tls",
	} {
		t.Run(ca, func(t *testing.T) {
			gin.SetMode(gin.ReleaseMode)
			router := gin.New()
			sent := false

			router.GET("/stream.m3u8", func(ctx *gin.Context) {
				if sent {
					return
				}
				sent = true

				ctx.Writer.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
				io.Copy(ctx.Writer, bytes.NewReader([]byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-ALLOW-CACHE:NO
#EXT-X-TARGETDURATION:2
#EXT-X-MEDIA-SEQUENCE:0
#EXTINF:2,
segment.ts?key=val
#EXT-X-ENDLIST
`)))
			})

			router.GET("/segment.ts", func(ctx *gin.Context) {
				require.Equal(t, "val", ctx.Query("key"))
				ctx.Writer.Header().Set("Content-Type", `video/MP2T`)

				mux := astits.NewMuxer(context.Background(), ctx.Writer)

				err := mux.AddElementaryStream(astits.PMTElementaryStream{
					ElementaryPID: 256,
					StreamType:    astits.StreamTypeH264Video,
				})
				require.NoError(t, err)

				err = mux.AddElementaryStream(astits.PMTElementaryStream{
					ElementaryPID: 257,
					StreamType:    astits.StreamTypeAACAudio,
				})
				require.NoError(t, err)

				mux.SetPCRPID(256)

				_, err = mux.WriteTables()
				require.NoError(t, err)

				enc, _ := h264.AnnexBMarshal([][]byte{
					{7, 1, 2, 3}, // SPS
					{8},          // PPS
					{5},          // IDR
				})

				_, err = mux.WriteData(&astits.MuxerData{
					PID: 256,
					PES: &astits.PESData{
						Header: &astits.PESHeader{
							OptionalHeader: &astits.PESOptionalHeader{
								MarkerBits:      2,
								PTSDTSIndicator: astits.PTSDTSIndicatorBothPresent,
								PTS:             &astits.ClockReference{Base: 90000},                   // +1 sec
								DTS:             &astits.ClockReference{Base: 0x1FFFFFFFF - 90000 + 1}, // -1 sec
							},
							StreamID: 224, // = video
						},
						Data: enc,
					},
				})
				require.NoError(t, err)

				pkts := mpeg4audio.ADTSPackets{
					{
						Type:         2,
						SampleRate:   44100,
						ChannelCount: 2,
						AU:           []byte{1, 2, 3, 4},
					},
				}
				enc, err = pkts.Marshal()
				require.NoError(t, err)

				_, err = mux.WriteData(&astits.MuxerData{
					PID: 257,
					PES: &astits.PESData{
						Header: &astits.PESHeader{
							OptionalHeader: &astits.PESOptionalHeader{
								MarkerBits:      2,
								PTSDTSIndicator: astits.PTSDTSIndicatorOnlyPTS,
								PTS:             &astits.ClockReference{Base: 0x1FFFFFFFF - 90000 + 1},
							},
							StreamID: 192, // = audio
						},
						Data: enc,
					},
				})
				require.NoError(t, err)
			})

			ln, err := net.Listen("tcp", "localhost:5780")
			require.NoError(t, err)

			s := &http.Server{Handler: router}

			if ca == "tls" {
				go func() {
					serverCertFpath, err := writeTempFile(serverCert)
					if err != nil {
						panic(err)
					}
					defer os.Remove(serverCertFpath)

					serverKeyFpath, err := writeTempFile(serverKey)
					if err != nil {
						panic(err)
					}
					defer os.Remove(serverKeyFpath)

					s.ServeTLS(ln, serverCertFpath, serverKeyFpath)
				}()
			} else {
				go s.Serve(ln)
			}

			defer s.Shutdown(context.Background())

			packetRecv := make(chan struct{}, 2)

			prefix := "http"
			if ca == "tls" {
				prefix = "https"
			}

			var c *Client
			c = &Client{
				URI: prefix + "://localhost:5780/stream.m3u8",
				HTTPClient: &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				},
				OnTracks: func(tracks []*Track) error {
					require.Equal(t, []*Track{
						{
							Codec: &codecs.H264{},
						},
						{
							Codec: &codecs.MPEG4Audio{
								Config: mpeg4audio.AudioSpecificConfig{
									Type:         2,
									SampleRate:   44100,
									ChannelCount: 2,
								},
							},
						},
					}, tracks)

					c.OnDataH26x(tracks[0], func(pts time.Duration, dts time.Duration, au [][]byte) {
						require.Equal(t, 2*time.Second, pts)
						require.Equal(t, time.Duration(0), dts)
						require.Equal(t, [][]byte{
							{7, 1, 2, 3},
							{8},
							{5},
						}, au)
						packetRecv <- struct{}{}
					})

					c.OnDataMPEG4Audio(tracks[1], func(pts time.Duration, aus [][]byte) {
						require.Equal(t, 0*time.Second, pts)
						require.Equal(t, [][]byte{
							{1, 2, 3, 4},
						}, aus)
						packetRecv <- struct{}{}
					})

					return nil
				},
			}

			err = c.Start()
			require.NoError(t, err)

			for i := 0; i < 2; i++ {
				<-packetRecv
			}

			c.Close()
			<-c.Wait()
		})
	}
}

func TestClientFMP4(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	router.GET("/stream.m3u8", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
		io.Copy(ctx.Writer, bytes.NewReader([]byte(`#EXTM3U
#EXT-X-VERSION:7
#EXT-X-MEDIA-SEQUENCE:20
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-TARGETDURATION:2
#EXT-X-MAP:URI="init.mp4?key=val"
#EXTINF:2,
segment.mp4?key=val
#EXT-X-ENDLIST
`)))
	})

	router.GET("/init.mp4", func(ctx *gin.Context) {
		require.Equal(t, "val", ctx.Query("key"))
		ctx.Writer.Header().Set("Content-Type", `video/mp4`)
		err := mp4ToWriter(&fmp4.Init{
			Tracks: []*fmp4.InitTrack{
				{
					ID:        99,
					TimeScale: 90000,
					Codec: &fmp4.CodecH264{
						SPS: testSPS,
						PPS: testPPS,
					},
				},
				{
					ID:        98,
					TimeScale: 44100,
					Codec: &fmp4.CodecMPEG4Audio{
						Config: testConfig,
					},
				},
			},
		}, ctx.Writer)
		require.NoError(t, err)
	})

	router.GET("/segment.mp4", func(ctx *gin.Context) {
		require.Equal(t, "val", ctx.Query("key"))
		ctx.Writer.Header().Set("Content-Type", `video/mp4`)

		payload, _ := h264.AVCCMarshal([][]byte{
			{7, 1, 2, 3}, // SPS
			{8},          // PPS
			{5},          // IDR
		})

		err := mp4ToWriter(&fmp4.Part{
			Tracks: []*fmp4.PartTrack{
				{
					ID:       98,
					BaseTime: 44100 * 6,
					Samples: []*fmp4.PartSample{{
						Duration: 44100 / 30,
						Payload:  []byte{1, 2, 3, 4},
					}},
				},
				{
					ID:       99,
					BaseTime: 90000 * 6,
					Samples: []*fmp4.PartSample{{
						Duration:  90000 / 30,
						PTSOffset: 90000 * 2,
						Payload:   payload,
					}},
				},
			},
		}, ctx.Writer)
		require.NoError(t, err)
	})

	ln, err := net.Listen("tcp", "localhost:5780")
	require.NoError(t, err)

	s := &http.Server{Handler: router}
	go s.Serve(ln)
	defer s.Shutdown(context.Background())

	packetRecv := make(chan struct{}, 2)

	var c *Client
	c = &Client{
		URI: "http://localhost:5780/stream.m3u8",
		OnTracks: func(tracks []*Track) error {
			require.Equal(t, []*Track{
				{
					Codec: &codecs.H264{
						SPS: testSPS,
						PPS: testPPS,
					},
				},
				{
					Codec: &codecs.MPEG4Audio{
						Config: testConfig,
					},
				},
			}, tracks)

			c.OnDataH26x(tracks[0], func(pts time.Duration, dts time.Duration, au [][]byte) {
				require.Equal(t, 2*time.Second, pts)
				require.Equal(t, time.Duration(0), dts)
				require.Equal(t, [][]byte{
					{7, 1, 2, 3},
					{8},
					{5},
				}, au)
				packetRecv <- struct{}{}
			})

			c.OnDataMPEG4Audio(tracks[1], func(pts time.Duration, aus [][]byte) {
				require.Equal(t, 0*time.Second, pts)
				require.Equal(t, [][]byte{
					{1, 2, 3, 4},
				}, aus)
				packetRecv <- struct{}{}
			})

			return nil
		},
	}

	err = c.Start()
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		<-packetRecv
	}

	c.Close()
	<-c.Wait()
}

func TestClientFMP4MultiRenditions(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	router.GET("/index.m3u8", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
		io.Copy(ctx.Writer, bytes.NewReader([]byte(`#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aac",NAME="English",DEFAULT=YES,AUTOSELECT=YES,LANGUAGE="en",URI="audio.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=7680000,CODECS="avc1.640015,mp4a.40.5",AUDIO="aac"
video.m3u8
`)))
	})

	router.GET("/video.m3u8", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
		io.Copy(ctx.Writer, bytes.NewReader([]byte(`#EXTM3U
#EXT-X-VERSION:7
#EXT-X-MEDIA-SEQUENCE:20
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-TARGETDURATION:2
#EXT-X-MAP:URI="init_video.mp4"
#EXTINF:2,
segment_video.mp4
#EXT-X-ENDLIST
`)))
	})

	router.GET("/audio.m3u8", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
		io.Copy(ctx.Writer, bytes.NewReader([]byte(`#EXTM3U
#EXT-X-VERSION:7
#EXT-X-MEDIA-SEQUENCE:20
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-TARGETDURATION:2
#EXT-X-MAP:URI="init_audio.mp4"
#EXTINF:2,
segment_audio.mp4
#EXT-X-ENDLIST
`)))
	})

	router.GET("/init_video.mp4", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `video/mp4`)

		err := mp4ToWriter(&fmp4.Init{
			Tracks: []*fmp4.InitTrack{
				{
					ID:        1,
					TimeScale: 90000,
					Codec: &fmp4.CodecH264{
						SPS: testSPS,
						PPS: testPPS,
					},
				},
			},
		}, ctx.Writer)
		require.NoError(t, err)
	})

	router.GET("/init_audio.mp4", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `video/mp4`)

		err := mp4ToWriter(&fmp4.Init{
			Tracks: []*fmp4.InitTrack{
				{
					ID:        1,
					TimeScale: 44100,
					Codec: &fmp4.CodecMPEG4Audio{
						Config: testConfig,
					},
				},
			},
		}, ctx.Writer)
		require.NoError(t, err)
	})

	router.GET("/segment_video.mp4", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `video/mp4`)

		payload, _ := h264.AVCCMarshal([][]byte{
			{7, 1, 2, 3}, // SPS
			{8},          // PPS
			{5},          // IDR
		})

		err := mp4ToWriter(&fmp4.Part{
			Tracks: []*fmp4.PartTrack{
				{
					ID: 1,
					Samples: []*fmp4.PartSample{{
						Duration:  90000,
						PTSOffset: 90000 * 3,
						Payload:   payload,
					}},
				},
			},
		}, ctx.Writer)
		require.NoError(t, err)
	})

	router.GET("/segment_audio.mp4", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `video/mp4`)

		err := mp4ToWriter(&fmp4.Part{
			Tracks: []*fmp4.PartTrack{
				{
					ID: 1,
					Samples: []*fmp4.PartSample{{
						Duration: 44100,
						Payload:  []byte{1, 2, 3, 4},
					}},
				},
			},
		}, ctx.Writer)
		require.NoError(t, err)
	})

	ln, err := net.Listen("tcp", "localhost:5780")
	require.NoError(t, err)

	s := &http.Server{Handler: router}
	go s.Serve(ln)
	defer s.Shutdown(context.Background())

	packetRecv := make(chan struct{}, 2)
	tracksRecv := make(chan struct{}, 1)

	var c *Client
	c = &Client{
		URI: "http://localhost:5780/index.m3u8",
		OnTracks: func(tracks []*Track) error {
			close(tracksRecv)

			require.Equal(t, []*Track{
				{
					Codec: &codecs.H264{
						SPS: testSPS,
						PPS: testPPS,
					},
				},
				{
					Codec: &codecs.MPEG4Audio{
						Config: testConfig,
					},
				},
			}, tracks)

			c.OnDataH26x(tracks[0], func(pts time.Duration, dts time.Duration, au [][]byte) {
				require.Equal(t, 3*time.Second, pts)
				require.Equal(t, time.Duration(0), dts)
				require.Equal(t, [][]byte{
					{7, 1, 2, 3},
					{8},
					{5},
				}, au)
				packetRecv <- struct{}{}
			})

			c.OnDataMPEG4Audio(tracks[1], func(pts time.Duration, aus [][]byte) {
				require.Equal(t, 0*time.Second, pts)
				require.Equal(t, [][]byte{
					{1, 2, 3, 4},
				}, aus)
				packetRecv <- struct{}{}
			})

			return nil
		},
	}

	err = c.Start()
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		<-packetRecv
	}

	c.Close()
	<-c.Wait()
}

func TestClientErrorInvalidSequenceID(t *testing.T) {
	router := gin.New()
	firstPlaylist := true

	router.GET("/stream.m3u8", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)

		if firstPlaylist {
			firstPlaylist = false
			io.Copy(ctx.Writer, bytes.NewReader([]byte(
				`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-ALLOW-CACHE:NO
#EXT-X-TARGETDURATION:2
#EXT-X-MEDIA-SEQUENCE:2
#EXTINF:2,
segment1.ts
#EXTINF:2,
segment1.ts
#EXTINF:2,
segment1.ts
`)))
		} else {
			io.Copy(ctx.Writer, bytes.NewReader([]byte(
				`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-ALLOW-CACHE:NO
#EXT-X-TARGETDURATION:2
#EXT-X-MEDIA-SEQUENCE:4
#EXTINF:2,
segment1.ts
#EXTINF:2,
segment1.ts
#EXTINF:2,
segment1.ts
`)))
		}
	})

	router.GET("/segment1.ts", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `video/MP2T`)
		mpegtsSegment(t, ctx.Writer)
	})

	ln, err := net.Listen("tcp", "localhost:5780")
	require.NoError(t, err)

	s := &http.Server{Handler: router}
	go s.Serve(ln)
	defer s.Shutdown(context.Background())

	c := &Client{
		URI: "http://localhost:5780/stream.m3u8",
	}
	require.NoError(t, err)

	err = c.Start()
	require.NoError(t, err)

	err = <-c.Wait()
	require.EqualError(t, err, "next segment not found or not ready yet")

	c.Close()
}
