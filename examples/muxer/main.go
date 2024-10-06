package main

import (
	_ "embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

// This example shows how to:
// 1. generate a MPEG-TS/H264 stream with GStreamer
// 2. re-encode the stream into HLS and serve it with an HTTP server

//go:embed index.html
var index []byte

func findH264Track(r *mpegts.Reader) *mpegts.Track {
	for _, track := range r.Tracks() {
		if _, ok := track.Codec.(*mpegts.CodecH264); ok {
			return track
		}
	}
	return nil
}

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
	go s.ListenAndServe()

	// create a socket to receive MPEG-TS packets
	pc, err := net.ListenPacket("udp", "localhost:9000")
	if err != nil {
		panic(err)
	}
	defer pc.Close()

	log.Println("Waiting for a MPEG-TS/H264 stream on UDP port 9000 - you can send one with GStreamer:\n" +
		"gst-launch-1.0 videotestsrc ! video/x-raw,width=1920,height=1080" +
		" ! x264enc speed-preset=ultrafast bitrate=3000 key-int-max=60" +
		" ! video/x-h264,profile=high" +
		" ! mpegtsmux alignment=6 ! udpsink host=127.0.0.1 port=9000")

	// create a MPEG-TS reader
	r, err := mpegts.NewReader(mpegts.NewBufferedReader(newPacketConnReader(pc)))
	if err != nil {
		panic(err)
	}

	// find the H264 track
	track := findH264Track(r)
	if track == nil {
		panic(fmt.Errorf("H264 track not found"))
	}

	var timeDec *mpegts.TimeDecoder2

	// setup a callback that is called when a H264 access unit is received
	r.OnDataH264(track, func(rawPTS int64, _ int64, au [][]byte) error {
		// decode the time
		if timeDec == nil {
			timeDec = mpegts.NewTimeDecoder2(rawPTS)
		}
		pts := timeDec.Decode(rawPTS)

		// pass the access unit to the HLS muxer
		log.Printf("visit http://localhost:8080 - encoding access unit with PTS = %v", pts)
		mux.WriteH264(videoTrack, time.Now(), pts, au)

		return nil
	})

	// read from the MPEG-TS stream
	for {
		err := r.Read()
		if err != nil {
			panic(err)
		}
	}
}
