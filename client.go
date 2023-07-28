/*
Package gohlslib is a HLS client and muxer library for the Go programming language.

Examples are available at https://github.com/bluenviron/gohlslib/tree/main/examples
*/
package gohlslib

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const (
	clientMPEGTSEntryQueueSize        = 100
	clientFMP4MaxPartTracksPerSegment = 200
	clientLiveInitialDistance         = 3
	clientLiveMaxDistanceFromEnd      = 5
	clientMaxDTSRTCDiff               = 10 * time.Second
)

// ClientOnTracksFunc is the prototype of the function passed to OnTracks().
type ClientOnTracksFunc func([]*Track) error

// ClientOnDataH26xFunc is the prototype of the function passed to OnDataH26x().
type ClientOnDataH26xFunc func(pts time.Duration, dts time.Duration, au [][]byte)

// ClientOnDataMPEG4AudioFunc is the prototype of the function passed to OnDataMPEG4Audio().
type ClientOnDataMPEG4AudioFunc func(pts time.Duration, dts time.Duration, aus [][]byte)

// ClientOnDataOpusFunc is the prototype of the function passed to OnDataOpus().
type ClientOnDataOpusFunc func(pts time.Duration, dts time.Duration, packets [][]byte)

func clientAbsoluteURL(base *url.URL, relative string) (*url.URL, error) {
	u, err := url.Parse(relative)
	if err != nil {
		return nil, err
	}
	return base.ResolveReference(u), nil
}

// Client is a HLS client.
type Client struct {
	//
	// Parameters (all optional except URI)
	//
	// URI of the playlist.
	URI string
	// HTTP client.
	// It defaults to http.DefaultClient.
	HTTPClient *http.Client
	// function that receives log messages.
	// It defaults to log.Printf.
	Log LogFunc

	//
	// private
	//

	ctx         context.Context
	ctxCancel   func()
	onTracks    ClientOnTracksFunc
	onData      map[*Track]interface{}
	playlistURL *url.URL

	// out
	outErr chan error
}

// Start starts the client.
func (c *Client) Start() error {
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
	if c.Log == nil {
		c.Log = defaultLog
	}

	var err error
	c.playlistURL, err = url.Parse(c.URI)
	if err != nil {
		return err
	}

	c.ctx, c.ctxCancel = context.WithCancel(context.Background())

	c.onData = make(map[*Track]interface{})
	c.outErr = make(chan error, 1)

	go c.run()

	return nil
}

// Close closes all the Client resources.
func (c *Client) Close() {
	c.ctxCancel()
}

// Wait waits for any error of the Client.
func (c *Client) Wait() chan error {
	return c.outErr
}

// OnTracks sets a callback that is called when tracks are read.
func (c *Client) OnTracks(cb ClientOnTracksFunc) {
	c.onTracks = cb
}

// OnDataH26x sets a callback that is called when data from an H26x track is received.
func (c *Client) OnDataH26x(forma *Track, cb ClientOnDataH26xFunc) {
	c.onData[forma] = cb
}

// OnDataMPEG4Audio sets a callback that is called when data from a MPEG-4 Audio track is received.
func (c *Client) OnDataMPEG4Audio(forma *Track, cb ClientOnDataMPEG4AudioFunc) {
	c.onData[forma] = cb
}

// OnDataOpus sets a callback that is called when data from an Opus track is received.
func (c *Client) OnDataOpus(forma *Track, cb ClientOnDataOpusFunc) {
	c.onData[forma] = cb
}

func (c *Client) run() {
	c.outErr <- c.runInner()
}

func (c *Client) runInner() error {
	rp := newClientRoutinePool()

	dl := newClientDownloaderPrimary(
		c.playlistURL,
		c.HTTPClient,
		c.Log,
		rp,
		c.onTracks,
		c.onData,
	)
	rp.add(dl)

	select {
	case err := <-rp.errorChan():
		rp.close()
		return err

	case <-c.ctx.Done():
		rp.close()
		return fmt.Errorf("terminated")
	}
}
