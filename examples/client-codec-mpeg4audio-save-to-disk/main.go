// Package main contains an example.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

// This example shows how to
// 1. read a HLS stream
// 2. check if there's a MPEG-4 audio track
// 2. save the MPEG-4 audio track to disk in MPEG-TS format

func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

func findMPEG4AudioTrack(tracks []*gohlslib.Track) *gohlslib.Track {
	for _, track := range tracks {
		if _, ok := track.Codec.(*codecs.MPEG4Audio); ok {
			return track
		}
	}
	return nil
}

func main() {
	// setup client
	c := &gohlslib.Client{
		URI: "https://devstreaming-cdn.apple.com/videos/streaming/examples/img_bipbop_adv_example_fmp4/master.m3u8",
	}

	var m *mpegtsMuxer
	defer func() {
		if m != nil {
			m.close()
		}
	}()

	// called when tracks are parsed
	c.OnTracks = func(tracks []*gohlslib.Track) error {
		// find the MPEG-4 Audio track
		track := findMPEG4AudioTrack(tracks)
		if track == nil {
			return fmt.Errorf("MPEG-4 audio track not found")
		}

		// create the MPEG-TS muxer
		m = &mpegtsMuxer{
			fileName: "mystream.ts",
			config:   &track.Codec.(*codecs.MPEG4Audio).Config,
		}
		err := m.initialize()
		if err != nil {
			return err
		}

		// set a callback that is called when data is received
		c.OnDataMPEG4Audio(track, func(pts int64, aus [][]byte) {
			log.Printf("received access unit with pts = %v\n", pts)

			// send data to the MPEG-TS muxer
			err := m.writeMPEG4Audio(aus, multiplyAndDivide(pts, 90000, int64(time.Second)))
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
