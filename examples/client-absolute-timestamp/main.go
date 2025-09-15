// Package main contains an example.
package main

import (
	"log"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

// This example shows how to:
// 1. read a HLS stream
// 2. get absolute timestamp of incoming data

func main() {
	// setup client
	c := &gohlslib.Client{
		URI: "http://myserver/mystream/index.m3u8",
	}

	// called when tracks are parsed
	c.OnTracks = func(tracks []*gohlslib.Track) error {
		for _, track := range tracks {
			ttrack := track

			log.Printf("detected track with codec %T\n", track.Codec)

			// set a callback that is called when data is received
			switch track.Codec.(type) {
			case *codecs.AV1:
				c.OnDataAV1(track, func(pts int64, _ [][]byte) {
					ntp, ntpAvailable := c.AbsoluteTime(ttrack)
					log.Printf("received data from track %T, pts = %v, ntp available = %v, ntp = %v\n",
						ttrack.Codec, pts, ntpAvailable, ntp)
				})

			case *codecs.H264, *codecs.H265:
				c.OnDataH26x(track, func(pts int64, _ int64, _ [][]byte) {
					ntp, ntpAvailable := c.AbsoluteTime(ttrack)
					log.Printf("received data from track %T, pts = %v, ntp available = %v, ntp = %v\n",
						ttrack.Codec, pts, ntpAvailable, ntp)
				})

			case *codecs.MPEG4Audio:
				c.OnDataMPEG4Audio(track, func(pts int64, _ [][]byte) {
					ntp, ntpAvailable := c.AbsoluteTime(ttrack)
					log.Printf("received data from track %T, pts = %v, ntp available = %v, ntp = %v\n",
						ttrack.Codec, pts, ntpAvailable, ntp)
				})

			case *codecs.Opus:
				c.OnDataOpus(track, func(pts int64, _ [][]byte) {
					ntp, ntpAvailable := c.AbsoluteTime(ttrack)
					log.Printf("received data from track %T, pts = %v, ntp available = %v, ntp = %v\n",
						ttrack.Codec, pts, ntpAvailable, ntp)
				})
			}
		}
		return nil
	}

	// start reading
	err := c.Start()
	if err != nil {
		panic(err)
	}
	defer c.Close()

	// wait for a fatal error
	panic(c.Wait2())
}
