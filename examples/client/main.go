package main

import (
	"log"
	"time"

	"github.com/bluenviron/gohlslib"
)

// This example shows how to read a HLS stream.

func main() {
	// setup client.
	c := gohlslib.Client{
		URI: "https://myserver/mystream/index.m3u8",
	}

	// setup a hook that is called when tracks are parsed
	c.OnTracks(func(tracks []*gohlslib.Track) error {
		for _, track := range tracks {
			log.Printf("detected track with codec %T\n", track.Codec)

			// setup a hook that is called when data is received
			ttrack := track
			c.OnData(track, func(pts time.Duration, unit interface{}) {
				log.Printf("received data from track %T, pts = %v", ttrack, pts)
			})
		}
		return nil
	})

	// start reading
	err := c.Start()
	if err != nil {
		panic(err)
	}
	defer c.Close()

	// wait for a fatal error
	<-c.Wait()
}
