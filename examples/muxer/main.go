//go:build cgo

// Package main contains an example.
package main

import (
	_ "embed"
	"log"
	"net/http"
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

// This example shows how to:
// 1. generate dummy RGBA images.
// 2. encode images with H264.
// 3. convert the H264 stream into HLS and serve it with an HTTP server.

// This example requires the FFmpeg libraries, that can be installed with this command:
// apt install -y libavcodec-dev libswscale-dev gcc pkg-config

//go:embed index.html
var index []byte

func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

// handleIndex wraps an HTTP handler and serves the home page
func handleIndex(wrapped http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write(index)
			return
		}

		wrapped(w, r)
	}
}

func main() {
	videoTrack := &gohlslib.Track{
		Codec:     &codecs.H264{},
		ClockRate: 90000,
	}

	// create the HLS muxer
	mux := &gohlslib.Muxer{
		Tracks: []*gohlslib.Track{videoTrack},
	}
	err := mux.Start()
	if err != nil {
		panic(err)
	}

	// create an HTTP server and link it to the HLS muxer
	s := &http.Server{
		Addr:    ":8080",
		Handler: handleIndex(mux.Handle),
	}
	log.Println("HTTP server created on :8080")
	go s.ListenAndServe() //nolint:errcheck

	// setup RGBA -> H264 encoder
	h264enc := &h264Encoder{
		Width:  640,
		Height: 480,
		FPS:    5,
	}
	err = h264enc.initialize()
	if err != nil {
		panic(err)
	}
	defer h264enc.close()

	start := time.Now()

	// setup a ticker to sleep between frames
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		// get current timestamp
		pts := multiplyAndDivide(int64(time.Since(start)), 90000, int64(time.Second))

		// create a dummy image
		img := createDummyImage()

		// encode the image with H264
		au, pts, err := h264enc.encode(img, pts)
		if err != nil {
			panic(err)
		}

		// wait for a H264 access unit
		if au == nil {
			continue
		}

		log.Printf("visit http://localhost:8080 - encoding access unit with PTS = %v", pts)

		// pass the access unit to the HLS muxer
		err = mux.WriteH264(videoTrack, time.Now(), pts, au)
		if err != nil {
			panic(err)
		}
	}
}
