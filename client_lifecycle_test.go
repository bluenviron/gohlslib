package gohlslib_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	mp4codecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

const clientLifecycleTimeout = 5 * time.Second

func clientLifecycleReceive[T any](t *testing.T, ch <-chan T, description string) T {
	t.Helper()
	timer := time.NewTimer(clientLifecycleTimeout)
	defer timer.Stop()

	select {
	case value := <-ch:
		return value
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", description)
		var zero T
		return zero
	}
}

func clientLifecycleClose(t *testing.T, c *gohlslib.Client) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		c.Close()
		close(done)
	}()
	clientLifecycleReceive(t, done, "Close")
}

func clientLifecycleWait(c *gohlslib.Client) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- c.Wait2()
	}()
	return done
}

func clientLifecycleAssertRunning(t *testing.T, done <-chan error) {
	t.Helper()
	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()

	select {
	case err := <-done:
		t.Fatalf("Wait2 returned before all callbacks completed: %v", err)
	case <-timer.C:
	}
}

func clientLifecycleMedia(t *testing.T, video bool, payloads [][]byte) ([]byte, []byte) {
	t.Helper()
	clockRate := uint32(44100)
	var codec mp4codecs.Codec = &mp4codecs.MPEG4Audio{
		Config: mpeg4audio.AudioSpecificConfig{
			Type:          2,
			SampleRate:    44100,
			ChannelConfig: 2,
		},
	}
	if video {
		clockRate = 90000
		codec = &mp4codecs.H264{
			SPS: []byte{
				0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
				0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
				0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
			},
			PPS: []byte{0x01, 0x02, 0x03, 0x04},
		}
	}

	var initBuffer seekablebuffer.Buffer
	require.NoError(t, (&fmp4.Init{Tracks: []*fmp4.InitTrack{{
		ID: 1, TimeScale: clockRate, Codec: codec,
	}}}).Marshal(&initBuffer))

	samples := make([]*fmp4.Sample, len(payloads))
	for i, payload := range payloads {
		if video {
			var err error
			payload, err = h264.AVCC([][]byte{payload}).Marshal()
			require.NoError(t, err)
		}
		samples[i] = &fmp4.Sample{Duration: clockRate / 100, Payload: payload}
	}
	var segmentBuffer seekablebuffer.Buffer
	require.NoError(t, (&fmp4.Part{Tracks: []*fmp4.PartTrack{{
		ID: 1, Samples: samples,
	}}}).Marshal(&segmentBuffer))
	return initBuffer.Bytes(), segmentBuffer.Bytes()
}

func clientLifecyclePlaylist(name string) string {
	return "#EXTM3U\n" +
		"#EXT-X-VERSION:7\n" +
		"#EXT-X-TARGETDURATION:1\n" +
		"#EXT-X-PLAYLIST-TYPE:VOD\n" +
		"#EXT-X-MAP:URI=\"" + name + "_init.mp4\"\n" +
		"#EXTINF:1,\n" + name + ".mp4\n" +
		"#EXT-X-ENDLIST\n"
}

func clientLifecycleBlockedServer(t *testing.T, primary bool) (string, <-chan struct{}, <-chan struct{}) {
	t.Helper()
	init, _ := clientLifecycleMedia(t, false, [][]byte{{1}})
	requested := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	blockedPath := "/segment.mp4"
	if primary {
		blockedPath = "/index.m3u8"
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case blockedPath:
			// Block the response body, not just the response headers.
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			close(requested)
			select {
			case <-r.Context().Done():
				close(canceled)
			case <-release:
			}
		case "/index.m3u8":
			_, _ = io.WriteString(w, clientLifecyclePlaylist("segment"))
		case "/segment_init.mp4":
			_, _ = w.Write(init)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	return server.URL + "/index.m3u8", requested, canceled
}

func TestClientLifecycleCloseBlockedHTTP(t *testing.T) {
	for _, name := range []string{"primary", "descendant"} {
		t.Run(name, func(t *testing.T) {
			uri, requested, canceled := clientLifecycleBlockedServer(t, name == "primary")
			tracksReady := make(chan struct{})
			c := &gohlslib.Client{
				URI: uri,
				OnTracks: func(_ []*gohlslib.Track) error {
					close(tracksReady)
					return nil
				},
			}
			require.NoError(t, c.Start())
			t.Cleanup(func() { clientLifecycleClose(t, c) })
			done := clientLifecycleWait(c)

			clientLifecycleReceive(t, requested, "blocked HTTP request")
			if name == "descendant" {
				clientLifecycleReceive(t, tracksReady, "descendant track discovery")
			}
			clientLifecycleClose(t, c)
			clientLifecycleReceive(t, canceled, "HTTP request cancellation")
			require.EqualError(t, clientLifecycleReceive(t, done, "Wait2"), "terminated")
		})
	}
}

func TestClientLifecycleOnTracksError(t *testing.T) {
	uri, requested, canceled := clientLifecycleBlockedServer(t, false)
	sentinel := errors.New("OnTracks sentinel")
	tracksReady := make(chan struct{})
	returnError := make(chan struct{})
	c := &gohlslib.Client{
		URI: uri,
		OnTracks: func(_ []*gohlslib.Track) error {
			close(tracksReady)
			<-returnError
			return sentinel
		},
	}
	require.NoError(t, c.Start())
	t.Cleanup(func() { clientLifecycleClose(t, c) })
	// Also release OnTracks if an assertion fails before returning the error.
	released := false
	t.Cleanup(func() {
		if !released {
			close(returnError)
		}
	})
	done := clientLifecycleWait(c)

	clientLifecycleReceive(t, tracksReady, "OnTracks")
	clientLifecycleReceive(t, requested, "active descendant HTTP request")
	close(returnError)
	released = true
	require.Same(t, sentinel, clientLifecycleReceive(t, done, "Wait2"))
	clientLifecycleReceive(t, canceled, "descendant request cancellation")

	clientLifecycleClose(t, c)
	require.Same(t, sentinel, clientLifecycleReceive(t, clientLifecycleWait(c), "Wait2 after Close"))
}

func TestClientLifecycleStaggeredRenditionEOS(t *testing.T) {
	videoInit, videoSegment := clientLifecycleMedia(t, true, [][]byte{{5, 1}})
	englishInit, englishSegment := clientLifecycleMedia(t, false, [][]byte{{1}, {2}})
	spanishInit, spanishSegment := clientLifecycleMedia(t, false, [][]byte{{3}, {4}, {5}})
	media := map[string][]byte{
		"/index.m3u8": []byte("#EXTM3U\n" +
			"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"aac\",NAME=\"English\",DEFAULT=YES,URI=\"english.m3u8\"\n" +
			"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"aac\",NAME=\"Spanish\",URI=\"spanish.m3u8\"\n" +
			"#EXT-X-STREAM-INF:BANDWIDTH=7680000,CODECS=\"avc1.42c028,mp4a.40.2\",AUDIO=\"aac\"\n" +
			"video.m3u8\n"),
		"/video.m3u8":       []byte(clientLifecyclePlaylist("video")),
		"/english.m3u8":     []byte(clientLifecyclePlaylist("english")),
		"/spanish.m3u8":     []byte(clientLifecyclePlaylist("spanish")),
		"/video_init.mp4":   videoInit,
		"/english_init.mp4": englishInit,
		"/spanish_init.mp4": spanishInit,
		"/video.mp4":        videoSegment,
		"/english.mp4":      englishSegment,
		"/spanish.mp4":      spanishSegment,
	}
	spanishRequested := make(chan struct{})
	releaseSpanish := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/spanish.mp4" {
			close(spanishRequested)
			select {
			case <-releaseSpanish:
			case <-r.Context().Done():
				return
			}
		}
		if body, ok := media[r.URL.Path]; ok {
			_, _ = w.Write(body)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	spanishReleased := false
	t.Cleanup(func() {
		if !spanishReleased {
			close(releaseSpanish)
		}
	})

	videoData := make(chan [][]byte, 1)
	englishData := make(chan [][]byte, 2)
	spanishData := make(chan [][]byte, 3)
	finalCallback := make(chan struct{})
	releaseCallback := make(chan struct{})
	callbackReturned := make(chan struct{})
	c := &gohlslib.Client{URI: server.URL + "/index.m3u8"}
	c.OnTracks = func(tracks []*gohlslib.Track) error {
		for _, track := range tracks {
			switch track.Codec.(type) {
			case *codecs.H264:
				c.OnDataH26x(track, func(_ int64, _ int64, au [][]byte) {
					videoData <- au
				})
			case *codecs.MPEG4Audio:
				if track.Name == "English" {
					c.OnDataMPEG4Audio(track, func(_ int64, aus [][]byte) {
						englishData <- aus
					})
				} else {
					count := 0
					c.OnDataMPEG4Audio(track, func(_ int64, aus [][]byte) {
						spanishData <- aus
						count++
						if count == 3 {
							defer close(callbackReturned)
							close(finalCallback)
							<-releaseCallback
						}
					})
				}
			}
		}
		return nil
	}
	require.NoError(t, c.Start())
	t.Cleanup(func() { clientLifecycleClose(t, c) })
	callbackReleased := false
	t.Cleanup(func() {
		if !callbackReleased {
			close(releaseCallback)
		}
	})
	done := clientLifecycleWait(c)

	clientLifecycleReceive(t, spanishRequested, "staggered rendition request")
	require.Equal(t, [][]byte{{5, 1}}, clientLifecycleReceive(t, videoData, "video sample"))
	for _, payload := range []byte{1, 2} {
		require.Equal(t, [][]byte{{payload}}, clientLifecycleReceive(t, englishData, "English sample"))
	}
	clientLifecycleAssertRunning(t, done)
	close(releaseSpanish)
	spanishReleased = true

	clientLifecycleReceive(t, finalCallback, "final Spanish callback")
	for _, payload := range []byte{3, 4, 5} {
		require.Equal(t, [][]byte{{payload}}, clientLifecycleReceive(t, spanishData, "Spanish sample"))
	}
	clientLifecycleAssertRunning(t, done)
	close(releaseCallback)
	callbackReleased = true
	require.Same(t, gohlslib.ErrClientEOS, clientLifecycleReceive(t, done, "EOS after all callbacks"))
	clientLifecycleReceive(t, callbackReturned, "final callback return")
	require.Empty(t, videoData)
	require.Empty(t, englishData)
	require.Empty(t, spanishData)
}
