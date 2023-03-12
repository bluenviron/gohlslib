/*
Package gohlslib is a HLS client and muxer library for the Go programming language.
Examples are available at https://github.com/bluenviron/gohlslib/tree/master/examples
*/
package gohlslib

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"

	"github.com/bluenviron/gohlslib/pkg/logger"
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

// ClientLogger allows to receive log lines.
type ClientLogger interface {
	Log(level logger.Level, format string, args ...interface{})
}

// Client is a HLS client.
type Client struct {
	// URL of the playlist.
	URI string

	// if the playlist certificate is self-signed
	// or invalid, you can provide the fingerprint of the certificate in order to
	// validate it anyway. It can be obtained by running:
	// openssl s_client -connect source_ip:source_port </dev/null 2>/dev/null | sed -n '/BEGIN/,/END/p' > server.crt
	// openssl x509 -in server.crt -noout -fingerprint -sha256 | cut -d "=" -f2 | tr -d ':'
	Fingerprint string

	// logger that receives log messages.
	Logger ClientLogger

	ctx         context.Context
	ctxCancel   func()
	onTracks    func([]format.Format) error
	onData      map[format.Format]func(time.Duration, interface{})
	playlistURL *url.URL

	// out
	outErr chan error
}

// Start starts the client.
func (c *Client) Start() error {
	var err error
	c.playlistURL, err = url.Parse(c.URI)
	if err != nil {
		return err
	}

	c.ctx, c.ctxCancel = context.WithCancel(context.Background())

	c.onData = make(map[format.Format]func(time.Duration, interface{}))
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
func (c *Client) OnTracks(cb func([]format.Format) error) {
	c.onTracks = cb
}

// OnData sets a callback that is called when data arrives.
func (c *Client) OnData(forma format.Format, cb func(time.Duration, interface{})) {
	c.onData[forma] = cb
}

func (c *Client) run() {
	c.outErr <- c.runInner()
}

func (c *Client) runInner() error {
	rp := newClientRoutinePool()

	dl := newClientDownloaderPrimary(
		c.playlistURL,
		c.Fingerprint,
		c.Logger,
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
