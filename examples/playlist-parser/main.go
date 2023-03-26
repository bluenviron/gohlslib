package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/bluenviron/gohlslib/pkg/playlist"
)

// This example shows how to download and parse a HLS playlist.

func main() {
	// connect to the HTTP server of the playlist
	req, err := http.Get("http://amssamples.streaming.mediaservices.windows.net/91492735-c523-432b-ba01-faba6c2206a2/AzureMediaServicesPromo.ism/manifest(format=m3u8-aapl)")
	if err != nil {
		panic(err)
	}
	defer req.Body.Close()

	// download the playlist
	byts, err := io.ReadAll(req.Body)
	if err != nil {
		panic(err)
	}

	// parse the playlist
	pl, err := playlist.Unmarshal(byts)
	if err != nil {
		panic(err)
	}

	switch pl := pl.(type) {
	case *playlist.Multivariant:
		fmt.Println("playlist is a multivariant playlist")

		fmt.Println("variants:")
		for _, variant := range pl.Variants {
			fmt.Printf(" * Bandwidth: %d Resolution: %s URI: %s \n", variant.Bandwidth, variant.Resolution, variant.URI)
		}

		fmt.Println("renditions:")
		for _, rendition := range pl.Renditions {
			fmt.Printf(" * Type: %s GroupID: %s URI: %s\n", rendition.Type, rendition.GroupID, rendition.URI)
		}

	case *playlist.Media:
		fmt.Println("playlist is a media playlist")

		fmt.Println("segments:")
		for _, seg := range pl.Segments {
			fmt.Printf(" * Duration: %s URI: %s\n", seg.Duration, seg.URI)
		}
	}
}
