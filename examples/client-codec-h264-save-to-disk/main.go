package main

import (
	"fmt"
	"log"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"
)

// This example shows how to
// 1. read a HLS stream
// 2. check if there's a H264 track
// 3. save the H264 track to disk in MPEG-TS format

func findH264Track(tracks []*gohlslib.Track) *gohlslib.Track {
	for _, track := range tracks {
		if _, ok := track.Codec.(*codecs.H264); ok {
			return track
		}
	}
	return nil
}

func main() {
	// setup client
	c := &gohlslib.Client{
		URI: "http://myserver/mystream/index.m3u8",
	}

	// called when tracks are parsed
	c.OnTracks = func(tracks []*gohlslib.Track) error {
		// find the H264 track
		track := findH264Track(tracks)
		if track == nil {
			return fmt.Errorf("H264 track not found")
		}

		// create the MPEG-TS muxer
		m := &mpegtsMuxer{
			fileName: "mystream.ts",
			sps:      track.Codec.(*codecs.H264).SPS,
			pps:      track.Codec.(*codecs.H264).PPS,
		}
		err := m.initialize()
		if err != nil {
			return nil
		}

		// set a callback that is called when data is received
		c.OnDataH26x(track, func(pts time.Duration, dts time.Duration, au [][]byte) {
			log.Printf("received access unit with pts = %v\n", pts)

			// send data to the MPEG-TS muxer
			err := m.writeH264(au, pts)
			if err != nil {
				panic(err)
			}
		})

		return nil
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
