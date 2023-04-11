package main

import (
	_ "embed"
	"log"
	"net/http"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"
)

// This example shows how to:
// 1. generate a MPEG-TS/H264 stream with GStreamer
// 2. re-encode the stream into HLS and serve it with an HTTP server.

//go:embed index.html
var index []byte

// handleIndex wraps an HTTP handler and serves the home page
func handleIndex(wrapped http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(index))
			return
		}

		wrapped(w, r)
	}
}

func main() {
	// create MPEG-TS receiver
	receiver, err := newMPEGTSReceiver()
	if err != nil {
		panic(err)
	}

	// create the HLS muxer
	mux := &gohlslib.Muxer{
		VideoTrack: &gohlslib.Track{
			Codec: &codecs.H264{},
		},
	}
	err = mux.Start()
	if err != nil {
		panic(err)
	}

	// create an HTTP server and link it to the HLS muxer
	s := &http.Server{
		Addr:    ":8080",
		Handler: handleIndex(mux.Handle),
	}
	log.Println("HTTP server created on :8080")
	go s.ListenAndServe()

	log.Println("Waiting for a MPEG-TS/H264 stream on UDP port 9000 - you can send one with GStreamer:\n" +
		"gst-launch-1.0 videotestsrc ! video/x-raw,width=1920,height=1080" +
		" ! x264enc speed-preset=ultrafast bitrate=3000 key-int-max=60" +
		" ! video/x-h264,profile=high" +
		" ! mpegtsmux alignment=6 ! udpsink host=127.0.0.1 port=9000")

	for {
		// read a H264 access unit from the stream
		au, pts, err := receiver.Read()
		if err != nil {
			panic(err)
		}

		log.Printf("visit http://localhost:8080 - encoding access unit with PTS = %v", pts)

		// encode access unit in HLS format
		mux.WriteH26x(time.Now(), pts, au)
	}
}
