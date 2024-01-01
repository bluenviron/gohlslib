package main

import (
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"
)

// This example shows how to
// 1. read a HLS stream
// 2. check if there's a H264 track
// 3. decode H264 access units into RGBA frames
// 4. convert frames to JPEG images and save them on disk

// This example requires the FFmpeg libraries, that can be installed with this command:
// apt install -y libavformat-dev libswscale-dev gcc pkg-config

func findH264Track(tracks []*gohlslib.Track) *gohlslib.Track {
	for _, track := range tracks {
		if _, ok := track.Codec.(*codecs.H264); ok {
			return track
		}
	}
	return nil
}

func saveToFile(img image.Image) error {
	// create file
	fname := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10) + ".jpg"
	f, err := os.Create(fname)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	log.Println("saving", fname)

	// convert to jpeg
	return jpeg.Encode(f, img, &jpeg.Options{
		Quality: 60,
	})
}

func main() {
	// setup client
	c := &gohlslib.Client{
		URI: "https://myserver/mystream/index.m3u8",
	}

	// called when tracks are parsed
	c.OnTracks = func(tracks []*gohlslib.Track) error {
		// find the H264 track
		track := findH264Track(tracks)
		if track == nil {
			return fmt.Errorf("H264 track not found")
		}

		// create the H264 decoder
		frameDec := &h264Decoder{}
		err := frameDec.initialize()
		if err != nil {
			return err
		}

		// if SPS and PPS are present into the track, send them to the decoder
		if track.Codec.(*codecs.H264).SPS != nil {
			frameDec.decode(track.Codec.(*codecs.H264).SPS)
		}
		if track.Codec.(*codecs.H264).PPS != nil {
			frameDec.decode(track.Codec.(*codecs.H264).PPS)
		}

		saveCount := 0

		// set a callback that is called when data is received
		c.OnDataH26x(track, func(pts time.Duration, dts time.Duration, au [][]byte) {
			log.Printf("received access unit with pts = %v\n", pts)

			for _, nalu := range au {
				// convert NALUs into RGBA frames
				img, err := frameDec.decode(nalu)
				if err != nil {
					panic(err)
				}

				// wait for a frame
				if img == nil {
					continue
				}

				// convert frame to JPEG and save to file
				err = saveToFile(img)
				if err != nil {
					panic(err)
				}

				saveCount++
				if saveCount == 5 {
					log.Printf("saved 5 images, exiting")
					os.Exit(1)
				}
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
