package main

import (
	"log"
	"time"

	"github.com/vicon-security/gohlslib"
	"github.com/vicon-security/gohlslib/pkg/codecs"
)

// This example shows how to read a HLS stream.

func main() {
	// setup client
	var c *gohlslib.Client
	c = &gohlslib.Client{
		URI: "https://myserver/mystream/index.m3u8",

		// set a callback that is called when tracks are parsed
		OnTracks: func(tracks []*gohlslib.Track) error {
			for _, track := range tracks {
				ttrack := track

				log.Printf("detected track with codec %T\n", track.Codec)

				// set a callback that is called when data is received
				switch track.Codec.(type) {
				case *codecs.AV1:
					c.OnDataAV1(track, func(pts time.Duration, tu [][]byte) {
						log.Printf("received data from track %T, pts = %v", ttrack, pts)
					})

				case *codecs.H264, *codecs.H265:
					c.OnDataH26x(track, func(pts time.Duration, dts time.Duration, au [][]byte) {
						log.Printf("received data from track %T, pts = %v", ttrack, pts)
					})

				case *codecs.MPEG4Audio:
					c.OnDataMPEG4Audio(track, func(pts time.Duration, aus [][]byte) {
						log.Printf("received data from track %T, pts = %v", ttrack, pts)
					})

				case *codecs.Opus:
					c.OnDataOpus(track, func(pts time.Duration, packets [][]byte) {
						log.Printf("received data from track %T, pts = %v", ttrack, pts)
					})
				}
			}
			return nil
		},
	}

	// start reading
	err := c.Start()
	if err != nil {
		panic(err)
	}
	defer c.Close()

	// wait for a fatal error
	panic(<-c.Wait())
}
