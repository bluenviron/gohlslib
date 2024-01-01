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
	clientMaxTracksPerStream     = 10
	clientMPEGTSSampleQueueSize  = 100
	clientLiveInitialDistance    = 3
	clientLiveMaxDistanceFromEnd = 5
	clientMaxDTSRTCDiff          = 10 * time.Second
)

// ClientOnDownloadPrimaryPlaylistFunc is the prototype of Client.OnDownloadPrimaryPlaylist.
type ClientOnDownloadPrimaryPlaylistFunc func(url string)

// ClientOnDownloadStreamPlaylistFunc is the prototype of Client.OnDownloadStreamPlaylist.
type ClientOnDownloadStreamPlaylistFunc func(url string)

// ClientOnDownloadSegmentFunc is the prototype of Client.OnDownloadSegment.
type ClientOnDownloadSegmentFunc func(url string)

// ClientOnDecodeErrorFunc is the prototype of Client.OnDecodeError.
type ClientOnDecodeErrorFunc func(err error)

// ClientOnTracksFunc is the prototype of the function passed to OnTracks().
type ClientOnTracksFunc func([]*Track) error

// ClientOnDataAV1Func is the prototype of the function passed to OnDataAV1().
type ClientOnDataAV1Func func(pts time.Duration, tu [][]byte)

// ClientOnDataVP9Func is the prototype of the function passed to OnDataVP9().
type ClientOnDataVP9Func func(pts time.Duration, frame []byte)

// ClientOnDataH26xFunc is the prototype of the function passed to OnDataH26x().
type ClientOnDataH26xFunc func(pts time.Duration, dts time.Duration, au [][]byte)

// ClientOnDataMPEG4AudioFunc is the prototype of the function passed to OnDataMPEG4Audio().
type ClientOnDataMPEG4AudioFunc func(pts time.Duration, aus [][]byte)

// ClientOnDataOpusFunc is the prototype of the function passed to OnDataOpus().
type ClientOnDataOpusFunc func(pts time.Duration, packets [][]byte)

type clientOnStreamTracksFunc func(context.Context, clientStreamProcessor) bool

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
	// parameters (all optional except URI)
	//
	// URI of the playlist.
	URI string
	// HTTP client.
	// It defaults to http.DefaultClient.
	HTTPClient *http.Client

	//
	// callbacks (all optional)
	//
	// called when tracks are available.
	OnTracks ClientOnTracksFunc
	// called before downloading a primary playlist.
	OnDownloadPrimaryPlaylist ClientOnDownloadPrimaryPlaylistFunc
	// called before downloading a stream playlist.
	OnDownloadStreamPlaylist ClientOnDownloadStreamPlaylistFunc
	// called before downloading a segment.
	OnDownloadSegment ClientOnDownloadSegmentFunc
	// called when a non-fatal decode error occurs.
	OnDecodeError ClientOnDecodeErrorFunc

	//
	// private
	//

	ctx               context.Context
	ctxCancel         func()
	onData            map[*Track]interface{}
	playlistURL       *url.URL
	primaryDownloader *clientPrimaryDownloader

	// out
	outErr chan error
}

// Start starts the client.
func (c *Client) Start() error {
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
	if c.OnTracks == nil {
		c.OnTracks = func(_ []*Track) error {
			return nil
		}
	}
	if c.OnDownloadPrimaryPlaylist == nil {
		c.OnDownloadPrimaryPlaylist = func(u string) {
			log.Printf("downloading primary playlist %v", u)
		}
	}
	if c.OnDownloadStreamPlaylist == nil {
		c.OnDownloadStreamPlaylist = func(u string) {
			log.Printf("downloading stream playlist %v", u)
		}
	}
	if c.OnDownloadSegment == nil {
		c.OnDownloadSegment = func(u string) {
			log.Printf("downloading segment %v", u)
		}
	}
	if c.OnDecodeError == nil {
		c.OnDecodeError = func(err error) {
			log.Println(err.Error())
		}
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

// OnDataAV1 sets a callback that is called when data from an AV1 track is received.
func (c *Client) OnDataAV1(track *Track, cb ClientOnDataAV1Func) {
	c.onData[track] = cb
}

// OnDataVP9 sets a callback that is called when data from a VP9 track is received.
func (c *Client) OnDataVP9(track *Track, cb ClientOnDataVP9Func) {
	c.onData[track] = cb
}

// OnDataH26x sets a callback that is called when data from an H26x track is received.
func (c *Client) OnDataH26x(track *Track, cb ClientOnDataH26xFunc) {
	c.onData[track] = cb
}

// OnDataMPEG4Audio sets a callback that is called when data from a MPEG-4 Audio track is received.
func (c *Client) OnDataMPEG4Audio(track *Track, cb ClientOnDataMPEG4AudioFunc) {
	c.onData[track] = cb
}

// OnDataOpus sets a callback that is called when data from an Opus track is received.
func (c *Client) OnDataOpus(track *Track, cb ClientOnDataOpusFunc) {
	c.onData[track] = cb
}

// AbsoluteTime returns the absolute timestamp of a packet with given track and DTS.
func (c *Client) AbsoluteTime(track *Track, dts time.Duration) (time.Time, bool) {
	return c.primaryDownloader.ntp(track, dts)
}

func (c *Client) run() {
	c.outErr <- c.runInner()
}

func (c *Client) runInner() error {
	rp := &clientRoutinePool{}
	rp.initialize()

	c.primaryDownloader = &clientPrimaryDownloader{
		primaryPlaylistURL:        c.playlistURL,
		httpClient:                c.HTTPClient,
		onDownloadPrimaryPlaylist: c.OnDownloadPrimaryPlaylist,
		onDownloadStreamPlaylist:  c.OnDownloadStreamPlaylist,
		onDownloadSegment:         c.OnDownloadSegment,
		onDecodeError:             c.OnDecodeError,
		rp:                        rp,
		onTracks:                  c.OnTracks,
		onData:                    c.onData,
	}
	c.primaryDownloader.initialize()
	rp.add(c.primaryDownloader)

	select {
	case err := <-rp.errorChan():
		rp.close()
		return err

	case <-c.ctx.Done():
		rp.close()
		return fmt.Errorf("terminated")
	}
}
