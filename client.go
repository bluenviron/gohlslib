/*
Package gohlslib is a HLS client and muxer library for the Go programming language.

Examples are available at https://github.com/bluenviron/gohlslib/tree/main/examples
*/
package gohlslib

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

const (
	clientMPEGTSEntryQueueSize        = 100
	clientFMP4MaxPartTracksPerSegment = 200
	clientLiveStartingInvPosition     = 3
	clientLiveMaxInvPosition          = 5
	clientMaxDTSRTCDiff               = 10 * time.Second
)

func clientAbsoluteURL(base *url.URL, relative string) (*url.URL, error) {
	u, err := url.Parse(relative)
	if err != nil {
		return nil, err
	}
	return base.ResolveReference(u), nil
}

// LogLevel is a log level.
type LogLevel int

// Log levels.
const (
	LogLevelDebug LogLevel = iota + 1
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// LogFunc is the prototype of the log function.
type LogFunc func(level LogLevel, format string, args ...interface{})

func defaultLog(level LogLevel, format string, args ...interface{}) {
	log.Printf(format, args...)
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
	onTracks    func([]*Track) error
	onData      map[*Track]func(time.Duration, interface{})
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

	c.onData = make(map[*Track]func(time.Duration, interface{}))
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
func (c *Client) OnTracks(cb func([]*Track) error) {
	c.onTracks = cb
}

// OnData sets a callback that is called when data arrives.
func (c *Client) OnData(forma *Track, cb func(time.Duration, interface{})) {
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
