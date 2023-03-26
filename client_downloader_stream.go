package gohlslib

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"

	"github.com/bluenviron/gohlslib/pkg/playlist"
)

func findSegmentWithInvPosition(segments []*playlist.MediaSegment, invPos int) (*playlist.MediaSegment, int) {
	index := len(segments) - invPos
	if index < 0 {
		return nil, 0
	}

	return segments[index], index
}

func findSegmentWithID(seqNo int, segments []*playlist.MediaSegment, id int) (*playlist.MediaSegment, int, int) {
	index := int(int64(id) - int64(seqNo))
	if index < 0 || index >= len(segments) {
		return nil, 0, 0
	}

	return segments[index], index, len(segments) - index
}

type clientDownloaderStream struct {
	isLeading            bool
	httpClient           *http.Client
	playlistURL          *url.URL
	initialPlaylist      *playlist.Media
	log                  LogFunc
	rp                   *clientRoutinePool
	onStreamTracks       func(context.Context, []format.Format) bool
	onSetLeadingTimeSync func(clientTimeSync)
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool)
	onData               map[format.Format]func(time.Duration, interface{})

	curSegmentID *int
}

func newClientDownloaderStream(
	isLeading bool,
	httpClient *http.Client,
	playlistURL *url.URL,
	initialPlaylist *playlist.Media,
	log LogFunc,
	rp *clientRoutinePool,
	onStreamTracks func(context.Context, []format.Format) bool,
	onSetLeadingTimeSync func(clientTimeSync),
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool),
	onData map[format.Format]func(time.Duration, interface{}),
) *clientDownloaderStream {
	return &clientDownloaderStream{
		isLeading:            isLeading,
		httpClient:           httpClient,
		playlistURL:          playlistURL,
		initialPlaylist:      initialPlaylist,
		log:                  log,
		rp:                   rp,
		onStreamTracks:       onStreamTracks,
		onSetLeadingTimeSync: onSetLeadingTimeSync,
		onGetLeadingTimeSync: onGetLeadingTimeSync,
		onData:               onData,
	}
}

func (d *clientDownloaderStream) run(ctx context.Context) error {
	initialPlaylist := d.initialPlaylist
	d.initialPlaylist = nil
	if initialPlaylist == nil {
		var err error
		initialPlaylist, err = d.downloadPlaylist(ctx)
		if err != nil {
			return err
		}
	}

	segmentQueue := newClientSegmentQueue()

	if initialPlaylist.Map != nil && initialPlaylist.Map.URI != "" {
		byts, err := d.downloadSegment(
			ctx,
			initialPlaylist.Map.URI,
			initialPlaylist.Map.ByteRangeStart,
			initialPlaylist.Map.ByteRangeLength)
		if err != nil {
			return err
		}

		proc, err := newClientProcessorFMP4(
			ctx,
			d.isLeading,
			byts,
			segmentQueue,
			d.log,
			d.rp,
			d.onStreamTracks,
			d.onSetLeadingTimeSync,
			d.onGetLeadingTimeSync,
			d.onData,
		)
		if err != nil {
			return err
		}

		d.rp.add(proc)
	} else {
		proc := newClientProcessorMPEGTS(
			d.isLeading,
			segmentQueue,
			d.log,
			d.rp,
			d.onStreamTracks,
			d.onSetLeadingTimeSync,
			d.onGetLeadingTimeSync,
			d.onData,
		)
		d.rp.add(proc)
	}

	err := d.fillSegmentQueue(ctx, initialPlaylist, segmentQueue)
	if err != nil {
		return err
	}

	for {
		ok := segmentQueue.waitUntilSizeIsBelow(ctx, 1)
		if !ok {
			return fmt.Errorf("terminated")
		}

		pl, err := d.downloadPlaylist(ctx)
		if err != nil {
			return err
		}

		err = d.fillSegmentQueue(ctx, pl, segmentQueue)
		if err != nil {
			return err
		}
	}
}

func (d *clientDownloaderStream) downloadPlaylist(ctx context.Context) (*playlist.Media, error) {
	d.log(LogLevelDebug, "downloading stream playlist %s", d.playlistURL.String())

	pl, err := clientDownloadPlaylist(ctx, d.httpClient, d.playlistURL)
	if err != nil {
		return nil, err
	}

	plt, ok := pl.(*playlist.Media)
	if !ok {
		return nil, fmt.Errorf("invalid playlist")
	}

	return plt, nil
}

func (d *clientDownloaderStream) downloadSegment(
	ctx context.Context,
	uri string,
	start *uint64,
	length *uint64,
) ([]byte, error) {
	u, err := clientAbsoluteURL(d.playlistURL, uri)
	if err != nil {
		return nil, err
	}

	d.log(LogLevelDebug, "downloading segment %s", u)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	if length != nil {
		if start == nil {
			v := uint64(0)
			start = &v
		}
		req.Header.Add("Range", "bytes="+strconv.FormatUint(*start, 10)+"-"+strconv.FormatUint(*start+*length-1, 10))
	}

	res, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("bad status code: %d", res.StatusCode)
	}

	byts, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return byts, nil
}

func (d *clientDownloaderStream) fillSegmentQueue(
	ctx context.Context,
	pl *playlist.Media,
	segmentQueue *clientSegmentQueue,
) error {
	var seg *playlist.MediaSegment
	var segPos int

	if d.curSegmentID == nil {
		if !pl.Endlist { // live stream: start from clientLiveStartingInvPosition
			seg, segPos = findSegmentWithInvPosition(pl.Segments, clientLiveStartingInvPosition)
			if seg == nil {
				return fmt.Errorf("there aren't enough segments to fill the buffer")
			}
		} else { // VOD stream: start from beginning
			if len(pl.Segments) == 0 {
				return fmt.Errorf("no segments found")
			}
			seg = pl.Segments[0]
		}
	} else {
		var invPos int
		seg, segPos, invPos = findSegmentWithID(pl.MediaSequence, pl.Segments, *d.curSegmentID+1)
		if seg == nil {
			return fmt.Errorf("following segment not found or not ready yet")
		}

		d.log(LogLevelDebug, "segment inverse position: %d", invPos)

		if !pl.Endlist && invPos > clientLiveMaxInvPosition {
			return fmt.Errorf("playback is too late")
		}
	}

	v := pl.MediaSequence + segPos
	d.curSegmentID = &v

	byts, err := d.downloadSegment(ctx, seg.URI, seg.ByteRangeStart, seg.ByteRangeLength)
	if err != nil {
		return err
	}

	segmentQueue.push(byts)

	if pl.Endlist && pl.Segments[len(pl.Segments)-1] == seg {
		<-ctx.Done()
		return fmt.Errorf("stream has ended")
	}

	return nil
}
