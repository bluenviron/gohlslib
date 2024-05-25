package gohlslib

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4/seekablebuffer"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
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

func mustMarshalAVCC(au [][]byte) []byte {
	enc, err := h264.AVCCMarshal(au)
	if err != nil {
		panic(err)
	}
	return enc
}

type marshaler interface {
	Marshal(w io.WriteSeeker) error
}

func mp4ToWriter(i marshaler, w io.Writer) error {
	var buf seekablebuffer.Buffer
	err := i.Marshal(&buf)
	if err != nil {
		return err
	}

	_, err = w.Write(buf.Bytes())
	return err
}

func TestClient(t *testing.T) {
	for _, mode := range []string{"plain", "tls"} {
		for _, format := range []string{"mpegts", "fmp4"} {
			t.Run(mode+"_"+format, func(t *testing.T) {
				httpServ := &http.Server{
					Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if format == "mpegts" {
							switch {
							case r.Method == http.MethodGet && r.URL.Path == "/stream.m3u8":
								w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
								w.Write([]byte("#EXTM3U\n" +
									"#EXT-X-VERSION:3\n" +
									"#EXT-X-ALLOW-CACHE:NO\n" +
									"#EXT-X-TARGETDURATION:2\n" +
									"#EXT-X-MEDIA-SEQUENCE:0\n" +
									"#EXT-X-PLAYLIST-TYPE:VOD\n" +
									"#EXT-X-PROGRAM-DATE-TIME:2015-02-05T01:02:02Z\n" +
									"#EXTINF:1,\n" +
									"segment1.ts?key=val\n" +
									"#EXTINF:1,\n" +
									"segment2.ts\n" +
									"#EXT-X-ENDLIST\n"))

							case r.Method == http.MethodGet && r.URL.Path == "/segment1.ts":
								q, err := url.ParseQuery(r.URL.RawQuery)
								require.NoError(t, err)
								require.Equal(t, "val", q.Get("key"))
								w.Header().Set("Content-Type", `video/MP2T`)

								h264Track := &mpegts.Track{
									Codec: &mpegts.CodecH264{},
								}
								mpeg4audioTrack := &mpegts.Track{
									Codec: &mpegts.CodecMPEG4Audio{
										Config: mpeg4audio.AudioSpecificConfig{
											Type:         2,
											SampleRate:   44100,
											ChannelCount: 2,
										},
									},
								}
								mw := mpegts.NewWriter(w, []*mpegts.Track{h264Track, mpeg4audioTrack})

								err = mw.WriteH264(
									h264Track,
									90000,      // +1 sec
									8589844592, // -1 sec
									true,
									[][]byte{
										{7, 1, 2, 3}, // SPS
										{8},          // PPS
										{5},          // IDR
									},
								)
								require.NoError(t, err)

								err = mw.WriteH264(
									h264Track,
									90000+90000/30,
									8589844592+90000/30,
									false,
									[][]byte{
										{1, 4, 5, 6},
									},
								)
								require.NoError(t, err)

								err = mw.WriteMPEG4Audio(
									mpeg4audioTrack,
									8589844592,
									[][]byte{{1, 2, 3, 4}},
								)
								require.NoError(t, err)

								err = mw.WriteMPEG4Audio(
									mpeg4audioTrack,
									8589844592+90000/30,
									[][]byte{{5, 6, 7, 8}},
								)
								require.NoError(t, err)

							case r.Method == http.MethodGet && r.URL.Path == "/segment2.ts":
								q, err := url.ParseQuery(r.URL.RawQuery)
								require.NoError(t, err)
								require.Equal(t, "", q.Get("key"))
								w.Header().Set("Content-Type", `video/MP2T`)

								h264Track := &mpegts.Track{
									Codec: &mpegts.CodecH264{},
								}
								mpeg4audioTrack := &mpegts.Track{
									Codec: &mpegts.CodecMPEG4Audio{
										Config: mpeg4audio.AudioSpecificConfig{
											Type:         2,
											SampleRate:   44100,
											ChannelCount: 2,
										},
									},
								}
								mw := mpegts.NewWriter(w, []*mpegts.Track{h264Track, mpeg4audioTrack})

								err = mw.WriteH264(
									h264Track,
									8589844592+2*90000/30,
									8589844592+2*90000/30,
									true,
									[][]byte{
										{4},
									},
								)
								require.NoError(t, err)
							}
						} else {
							switch {
							case r.Method == http.MethodGet && r.URL.Path == "/stream.m3u8":
								w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
								w.Write([]byte("#EXTM3U\n" +
									"#EXT-X-VERSION:7\n" +
									"#EXT-X-MEDIA-SEQUENCE:20\n" +
									"#EXT-X-PLAYLIST-TYPE:VOD\n" +
									"#EXT-X-INDEPENDENT-SEGMENTS\n" +
									"#EXT-X-TARGETDURATION:2\n" +
									"#EXT-X-MAP:URI=\"init.mp4?key=val\"\n" +
									"#EXT-X-PROGRAM-DATE-TIME:2015-02-05T01:02:02Z\n" +
									"#EXTINF:2,\n" +
									"segment1.mp4?key=val\n" +
									"#EXTINF:2,\n" +
									"segment2.mp4\n" +
									"#EXT-X-ENDLIST\n"))

							case r.Method == http.MethodGet && r.URL.Path == "/init.mp4":
								q, err := url.ParseQuery(r.URL.RawQuery)
								require.NoError(t, err)
								require.Equal(t, "val", q.Get("key"))
								w.Header().Set("Content-Type", `video/mp4`)
								err = mp4ToWriter(&fmp4.Init{
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
								}, w)
								require.NoError(t, err)

							case r.Method == http.MethodGet && r.URL.Path == "/segment1.mp4":
								q, err := url.ParseQuery(r.URL.RawQuery)
								require.NoError(t, err)
								require.Equal(t, "val", q.Get("key"))
								w.Header().Set("Content-Type", `video/mp4`)

								err = mp4ToWriter(&fmp4.Part{
									Tracks: []*fmp4.PartTrack{
										{
											ID:       98,
											BaseTime: 44100 * 6,
											Samples: []*fmp4.PartSample{
												{
													Duration: 44100 / 30,
													Payload:  []byte{1, 2, 3, 4},
												},
												{
													Duration: 44100 / 30,
													Payload:  []byte{5, 6, 7, 8},
												},
											},
										},
										{
											ID:       99,
											BaseTime: 90000 * 6,
											Samples: []*fmp4.PartSample{
												{
													Duration:  90000 / 30,
													PTSOffset: 90000 * 2,
													Payload: mustMarshalAVCC([][]byte{
														{7, 1, 2, 3}, // SPS
														{8},          // PPS
														{5},          // IDR
													}),
												},
												{
													Duration:  90000 / 30,
													PTSOffset: 90000 * 2,
													Payload: mustMarshalAVCC([][]byte{
														{1, 4, 5, 6},
													}),
												},
											},
										},
									},
								}, w)
								require.NoError(t, err)

							case r.Method == http.MethodGet && r.URL.Path == "/segment2.mp4":
								q, err := url.ParseQuery(r.URL.RawQuery)
								require.NoError(t, err)
								require.Equal(t, "", q.Get("key"))
								w.Header().Set("Content-Type", `video/mp4`)

								err = mp4ToWriter(&fmp4.Part{
									Tracks: []*fmp4.PartTrack{
										{
											ID:       99,
											BaseTime: 90000*6 + 2*90000/30,
											Samples: []*fmp4.PartSample{
												{
													Duration:  90000 / 30,
													PTSOffset: 0,
													Payload: mustMarshalAVCC([][]byte{
														{4},
													}),
												},
											},
										},
									},
								}, w)
								require.NoError(t, err)
							}
						}
					}),
				}

				ln, err := net.Listen("tcp", "localhost:5780")
				require.NoError(t, err)

				if mode == "tls" {
					go func() {
						serverCertFpath, err2 := writeTempFile(serverCert)
						if err2 != nil {
							panic(err2)
						}
						defer os.Remove(serverCertFpath)

						serverKeyFpath, err2 := writeTempFile(serverKey)
						if err2 != nil {
							panic(err2)
						}
						defer os.Remove(serverKeyFpath)

						httpServ.ServeTLS(ln, serverCertFpath, serverKeyFpath)
					}()
				} else {
					go httpServ.Serve(ln)
				}

				defer httpServ.Shutdown(context.Background())

				videoRecv := make(chan struct{})
				audioRecv := make(chan struct{})
				videoCount := 0
				audioCount := 0

				prefix := "http"
				if mode == "tls" {
					prefix = "https"
				}

				tr := &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
				}
				defer tr.CloseIdleConnections()

				var c *Client
				c = &Client{
					URI:        prefix + "://localhost:5780/stream.m3u8",
					HTTPClient: &http.Client{Transport: tr},
					OnTracks: func(tracks []*Track) error {
						var sps []byte
						var pps []byte
						if format == "fmp4" {
							sps = testSPS
							pps = testPPS
						}

						require.Equal(t, []*Track{
							{
								Codec: &codecs.H264{
									SPS: sps,
									PPS: pps,
								},
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
							switch videoCount {
							case 0:
								require.Equal(t, time.Duration(0), dts)
								require.Equal(t, 2*time.Second, pts)
								require.Equal(t, [][]byte{
									{7, 1, 2, 3},
									{8},
									{5},
								}, au)
								ntp, ok := c.AbsoluteTime(tracks[0])
								require.Equal(t, true, ok)
								require.Equal(t, time.Date(2015, time.February, 5, 1, 2, 2, 0, time.UTC), ntp)

							case 1:
								require.Equal(t, 33333333*time.Nanosecond, dts)
								require.Equal(t, 2*time.Second+33333333*time.Nanosecond, pts)
								require.Equal(t, [][]byte{{1, 4, 5, 6}}, au)
								ntp, ok := c.AbsoluteTime(tracks[0])
								require.Equal(t, true, ok)
								require.Equal(t, time.Date(2015, time.February, 5, 1, 2, 2, 33333333, time.UTC), ntp)

							case 2:
								require.Equal(t, 66666666*time.Nanosecond, dts)
								require.Equal(t, 66666666*time.Nanosecond, pts)
								require.Equal(t, [][]byte{{4}}, au)
								_, ok := c.AbsoluteTime(tracks[0])
								require.Equal(t, false, ok)
								close(videoRecv)
							}
							videoCount++
						})

						c.OnDataMPEG4Audio(tracks[1], func(pts time.Duration, aus [][]byte) {
							switch audioCount {
							case 0:
								require.Equal(t, 0*time.Second, pts)
								require.Equal(t, [][]byte{{1, 2, 3, 4}}, aus)
								ntp, ok := c.AbsoluteTime(tracks[1])
								require.Equal(t, true, ok)
								require.Equal(t, time.Date(2015, time.February, 5, 1, 2, 2, 0, time.UTC), ntp)

							case 1:
								require.Equal(t, 33333333*time.Nanosecond, pts)
								require.Equal(t, [][]byte{{5, 6, 7, 8}}, aus)
								ntp, ok := c.AbsoluteTime(tracks[1])
								require.Equal(t, true, ok)
								require.Equal(t, time.Date(2015, time.February, 5, 1, 2, 2, 33333333, time.UTC), ntp)
								close(audioRecv)
							}
							audioCount++
						})

						return nil
					},
				}

				err = c.Start()
				require.NoError(t, err)

				<-videoRecv
				<-audioRecv

				err = <-c.Wait()
				require.Equal(t, ErrClientEOS, err)

				c.Close()
			})
		}
	}
}

func TestClientFMP4MultiRenditions(t *testing.T) {
	httpServ := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/index.m3u8":
				w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
				w.Write([]byte("#EXTM3U\n" +
					"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"aac\",NAME=\"English\"," +
					"DEFAULT=YES,AUTOSELECT=YES,LANGUAGE=\"en\",URI=\"audio.m3u8\"\n" +
					"#EXT-X-STREAM-INF:BANDWIDTH=7680000,CODECS=\"avc1.640015,mp4a.40.5\",AUDIO=\"aac\"\n" +
					"video.m3u8\n"))

			case r.Method == http.MethodGet && r.URL.Path == "/video.m3u8":
				w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
				w.Write([]byte("#EXTM3U\n" +
					"#EXT-X-VERSION:7\n" +
					"#EXT-X-MEDIA-SEQUENCE:20\n" +
					"#EXT-X-PLAYLIST-TYPE:VOD\n" +
					"#EXT-X-INDEPENDENT-SEGMENTS\n" +
					"#EXT-X-TARGETDURATION:2\n" +
					"#EXT-X-MAP:URI=\"init_video.mp4\"\n" +
					"#EXTINF:2,\n" +
					"segment_video.mp4\n" +
					"#EXT-X-ENDLIST\n"))

			case r.Method == http.MethodGet && r.URL.Path == "/audio.m3u8":
				w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
				w.Write([]byte("#EXTM3U\n" +
					"#EXT-X-VERSION:7\n" +
					"#EXT-X-MEDIA-SEQUENCE:20\n" +
					"#EXT-X-PLAYLIST-TYPE:VOD\n" +
					"#EXT-X-INDEPENDENT-SEGMENTS\n" +
					"#EXT-X-TARGETDURATION:2\n" +
					"#EXT-X-MAP:URI=\"init_audio.mp4\"\n" +
					"#EXTINF:2,\n" +
					"segment_audio.mp4\n" +
					"#EXT-X-ENDLIST"))

			case r.Method == http.MethodGet && r.URL.Path == "/init_video.mp4":
				w.Header().Set("Content-Type", `video/mp4`)
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
				}, w)
				require.NoError(t, err)

			case r.Method == http.MethodGet && r.URL.Path == "/init_audio.mp4":
				w.Header().Set("Content-Type", `video/mp4`)
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
				}, w)
				require.NoError(t, err)

			case r.Method == http.MethodGet && r.URL.Path == "/segment_video.mp4":
				w.Header().Set("Content-Type", `video/mp4`)
				err := mp4ToWriter(&fmp4.Part{
					Tracks: []*fmp4.PartTrack{
						{
							ID: 1,
							Samples: []*fmp4.PartSample{{
								Duration:  90000,
								PTSOffset: 90000 * 3,
								Payload: mustMarshalAVCC([][]byte{
									{7, 1, 2, 3}, // SPS
									{8},          // PPS
									{5},          // IDR
								}),
							}},
						},
					},
				}, w)
				require.NoError(t, err)

			case r.Method == http.MethodGet && r.URL.Path == "/segment_audio.mp4":
				w.Header().Set("Content-Type", `video/mp4`)
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
				}, w)
				require.NoError(t, err)
			}
		}),
	}

	ln, err := net.Listen("tcp", "localhost:5780")
	require.NoError(t, err)

	go httpServ.Serve(ln)
	defer httpServ.Shutdown(context.Background())

	packetRecv := make(chan struct{}, 2)
	tracksRecv := make(chan struct{}, 1)

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()

	var c *Client
	c = &Client{
		URI:        "http://localhost:5780/index.m3u8",
		HTTPClient: &http.Client{Transport: tr},
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

	<-c.Wait()
	c.Close()
}

func TestClientFMP4LowLatency(t *testing.T) {
	count := 0
	closeRequest := make(chan struct{})

	httpServ := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/stream.m3u8":
				w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)

				switch count {
				case 0:
					q, err := url.ParseQuery(r.URL.RawQuery)
					require.NoError(t, err)
					require.Equal(t, "", q.Get("_HLS_skip"))
					w.Write([]byte("#EXTM3U\n" +
						"#EXT-X-VERSION:9\n" +
						"#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=5.00000,CAN-SKIP-UNTIL=24.00000\n" +
						"#EXT-X-MEDIA-SEQUENCE:20\n" +
						"#EXT-X-TARGETDURATION:2\n" +
						"#EXT-X-MAP:URI=\"init.mp4\"\n" +
						"#EXT-X-PROGRAM-DATE-TIME:2015-02-05T01:02:02Z\n" +
						"#EXTINF:2,\n" +
						"segment.mp4\n" +
						"#EXT-X-PRELOAD-HINT:TYPE=PART,URI=part1.mp4\n"))

				case 1:
					q, err := url.ParseQuery(r.URL.RawQuery)
					require.NoError(t, err)
					require.Equal(t, "YES", q.Get("_HLS_skip"))
					w.Write([]byte("#EXTM3U\n" +
						"#EXT-X-VERSION:9\n" +
						"#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=5.00000,CAN-SKIP-UNTIL=24.00000\n" +
						"#EXT-X-MEDIA-SEQUENCE:20\n" +
						"#EXT-X-TARGETDURATION:2\n" +
						"#EXT-X-MAP:URI=\"init.mp4\"\n" +
						"#EXT-X-PROGRAM-DATE-TIME:2015-02-05T01:02:02Z\n" +
						"#EXTINF:2,\n" +
						"segment.mp4\n" +
						"#EXT-X-PART:DURATION=0.066666666,URI=\"part1.mp4\",INDEPENDENT=YES\n" +
						"#EXT-X-PRELOAD-HINT:TYPE=PART,URI=part2.mp4\n"))

				case 2:
					q, err := url.ParseQuery(r.URL.RawQuery)
					require.NoError(t, err)
					require.Equal(t, "YES", q.Get("_HLS_skip"))
					w.Write([]byte("#EXTM3U\n" +
						"#EXT-X-VERSION:9\n" +
						"#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=5.00000,CAN-SKIP-UNTIL=24.00000\n" +
						"#EXT-X-MEDIA-SEQUENCE:20\n" +
						"#EXT-X-TARGETDURATION:2\n" +
						"#EXT-X-MAP:URI=\"init.mp4\"\n" +
						"#EXT-X-PROGRAM-DATE-TIME:2015-02-05T01:02:02Z\n" +
						"#EXTINF:2,\n" +
						"segment.mp4\n" +
						"#EXT-X-PART:DURATION=0.066666666,URI=\"part1.mp4\",INDEPENDENT=YES\n" +
						"#EXT-X-PART:DURATION=0.033333333,URI=\"part2.mp4\"\n" +
						"#EXT-X-PRELOAD-HINT:TYPE=PART,URI=part3.mp4\n"))
				}
				count++

			case r.Method == http.MethodGet && r.URL.Path == "/init.mp4":
				w.Header().Set("Content-Type", `video/mp4`)
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
				}, w)
				require.NoError(t, err)

			case r.Method == http.MethodGet && r.URL.Path == "/part1.mp4":
				w.Header().Set("Content-Type", `video/mp4`)
				err := mp4ToWriter(&fmp4.Part{
					Tracks: []*fmp4.PartTrack{
						{
							ID: 1,
							Samples: []*fmp4.PartSample{
								{
									Duration: 90000 / 30,
									Payload: mustMarshalAVCC([][]byte{
										{7, 1, 2, 3}, // SPS
										{8},          // PPS
										{5},          // IDR
									}),
								},
								{
									Duration: 90000 / 30,
									Payload: mustMarshalAVCC([][]byte{
										{1, 4, 5, 6},
									}),
								},
							},
						},
					},
				}, w)
				require.NoError(t, err)

			case r.Method == http.MethodGet && r.URL.Path == "/part2.mp4":
				w.Header().Set("Content-Type", `video/mp4`)
				err := mp4ToWriter(&fmp4.Part{
					Tracks: []*fmp4.PartTrack{
						{
							ID:       1,
							BaseTime: (90000 / 30) * 2,
							Samples: []*fmp4.PartSample{{
								Duration: 90000 / 30,
								Payload: mustMarshalAVCC([][]byte{
									{1, 7, 8, 9},
								}),
							}},
						},
					},
				}, w)
				require.NoError(t, err)

			case r.Method == http.MethodGet && r.URL.Path == "/part3.mp4":
				<-closeRequest
			}
		}),
	}

	ln, err := net.Listen("tcp", "localhost:5780")
	require.NoError(t, err)

	go httpServ.Serve(ln)
	defer httpServ.Shutdown(context.Background())

	packetRecv := make(chan struct{})
	recvCount := 0

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()

	var c *Client
	c = &Client{
		URI:        "http://localhost:5780/stream.m3u8",
		HTTPClient: &http.Client{Transport: tr},
		OnTracks: func(tracks []*Track) error {
			require.Equal(t, []*Track{
				{
					Codec: &codecs.H264{
						SPS: testSPS,
						PPS: testPPS,
					},
				},
			}, tracks)

			c.OnDataH26x(tracks[0], func(pts time.Duration, dts time.Duration, au [][]byte) {
				switch recvCount {
				case 0:
					ntp, ok := c.AbsoluteTime(tracks[0])
					require.Equal(t, true, ok)
					require.Equal(t, time.Date(2015, time.February, 5, 1, 2, 4, 0, time.UTC), ntp)
					require.Equal(t, 0*time.Second, pts)
					require.Equal(t, time.Duration(0), dts)
					require.Equal(t, [][]byte{
						{7, 1, 2, 3},
						{8},
						{5},
					}, au)

				case 1:
					ntp, ok := c.AbsoluteTime(tracks[0])
					require.Equal(t, true, ok)
					require.Equal(t, time.Date(2015, time.February, 5, 1, 2, 4, 33333333, time.UTC), ntp)
					require.Equal(t, 33333333*time.Nanosecond, pts)
					require.Equal(t, 33333333*time.Nanosecond, dts)
					require.Equal(t, [][]byte{{1, 4, 5, 6}}, au)

				case 2:
					ntp, ok := c.AbsoluteTime(tracks[0])
					require.Equal(t, true, ok)
					require.Equal(t, time.Date(2015, time.February, 5, 1, 2, 4, 66666666, time.UTC), ntp)
					require.Equal(t, 66666666*time.Nanosecond, pts)
					require.Equal(t, 66666666*time.Nanosecond, dts)
					require.Equal(t, [][]byte{{1, 7, 8, 9}}, au)

				default:
					t.Errorf("should not happen")
				}
				recvCount++
				packetRecv <- struct{}{}
			})

			return nil
		},
	}

	err = c.Start()
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		<-packetRecv
	}

	c.Close()
	<-c.Wait()

	close(closeRequest)
}

func TestClientErrorInvalidSequenceID(t *testing.T) {
	first := true

	httpServ := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/stream.m3u8" {
				w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
				if first {
					first = false
					w.Write([]byte("#EXTM3U\n" +
						"#EXT-X-VERSION:3\n" +
						"#EXT-X-ALLOW-CACHE:NO\n" +
						"#EXT-X-TARGETDURATION:2\n" +
						"#EXT-X-MEDIA-SEQUENCE:2\n" +
						"#EXTINF:2,\n" +
						"segment1.ts\n" +
						"#EXTINF:2,\n" +
						"segment1.ts\n" +
						"#EXTINF:2,\n" +
						"segment1.ts\n"))
				} else {
					w.Write([]byte("#EXTM3U\n" +
						"#EXT-X-VERSION:3\n" +
						"#EXT-X-ALLOW-CACHE:NO\n" +
						"#EXT-X-TARGETDURATION:2\n" +
						"#EXT-X-MEDIA-SEQUENCE:4\n" +
						"#EXTINF:2,\n" +
						"segment1.ts\n" +
						"#EXTINF:2,\n" +
						"segment1.ts\n" +
						"#EXTINF:2,\n" +
						"segment1.ts\n"))
				}
			} else if r.Method == http.MethodGet && r.URL.Path == "/segment1.ts" {
				w.Header().Set("Content-Type", `video/MP2T`)

				h264Track := &mpegts.Track{
					Codec: &mpegts.CodecH264{},
				}
				mw := mpegts.NewWriter(w, []*mpegts.Track{h264Track})

				err := mw.WriteH264(
					h264Track,
					90000,               // +1 sec
					0x1FFFFFFFF-90000+1, // -1 sec
					true,
					[][]byte{
						{7, 1, 2, 3}, // SPS
						{8},          // PPS
						{5},          // IDR
					},
				)
				require.NoError(t, err)
			}
		}),
	}

	ln, err := net.Listen("tcp", "localhost:5780")
	require.NoError(t, err)

	go httpServ.Serve(ln)
	defer httpServ.Shutdown(context.Background())

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()

	c := &Client{
		URI:        "http://localhost:5780/stream.m3u8",
		HTTPClient: &http.Client{Transport: tr},
	}
	require.NoError(t, err)

	err = c.Start()
	require.NoError(t, err)

	err = <-c.Wait()
	require.EqualError(t, err, "next segment not found or not ready yet")

	c.Close()
}
