package gohlslib

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist"
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

func dateTimeOfPreloadHint(pl *playlist.Media) *time.Time {
	if len(pl.Segments) == 0 {
		return nil
	}

	lastSeg := pl.Segments[len(pl.Segments)-1]
	if lastSeg.DateTime == nil {
		return nil
	}

	d := lastSeg.DateTime.Add(lastSeg.Duration)

	for _, part := range pl.Parts {
		d = d.Add(part.Duration)
	}

	return &d
}

type clientStreamDownloaderClient interface {
	setTimeConv(ts clientTimeConv)
	waitTimeConv(ctx context.Context) (clientTimeConv, bool)
}

type clientStreamDownloader struct {
	isLeading                bool
	startDistance            int
	maxDistance              int
	httpClient               *http.Client
	onRequest                ClientOnRequestFunc
	onDownloadStreamPlaylist ClientOnDownloadStreamPlaylistFunc
	onDownloadSegment        ClientOnDownloadSegmentFunc
	onDownloadPart           ClientOnDownloadPartFunc
	onDecodeError            ClientOnDecodeErrorFunc
	playlistURL              *url.URL
	rendition                *playlist.MultivariantRendition
	firstPlaylist            *playlist.Media
	rp                       *clientRoutinePool
	client                   clientStreamDownloaderClient

	segmentQueue *clientSegmentQueue
	curSegmentID *int

	// out
	chTracks         chan []*Track
	chProcessorError chan error

	// in
	chStartStreaming chan map[*Track]*clientTrack
}

func (d *clientStreamDownloader) initialize() {
	d.chTracks = make(chan []*Track)
	d.chProcessorError = make(chan error)
	d.chStartStreaming = make(chan map[*Track]*clientTrack)
}

func (d *clientStreamDownloader) run(ctx context.Context) error {
	if d.firstPlaylist == nil {
		var err error
		d.firstPlaylist, err = d.downloadPlaylist(ctx, false)
		if err != nil {
			return err
		}
	}

	d.segmentQueue = &clientSegmentQueue{}
	d.segmentQueue.initialize()

	if d.firstPlaylist.Map != nil && d.firstPlaylist.Map.URI != "" {
		initFile, err := d.downloadSegment(
			ctx,
			d.firstPlaylist.Map.URI,
			d.firstPlaylist.Map.ByteRangeStart,
			d.firstPlaylist.Map.ByteRangeLength)
		if err != nil {
			return err
		}

		proc := &clientStreamProcessorFMP4{
			ctx:              ctx,
			isLeading:        d.isLeading,
			rendition:        d.rendition,
			initFile:         initFile,
			segmentQueue:     d.segmentQueue,
			rp:               d.rp,
			streamDownloader: d,
			client:           d.client,
		}
		proc.initialize()
		d.rp.add(proc)
	} else {
		proc := &clientStreamProcessorMPEGTS{
			onDecodeError:    d.onDecodeError,
			isLeading:        d.isLeading,
			segmentQueue:     d.segmentQueue,
			rp:               d.rp,
			streamDownloader: d,
			client:           d.client,
		}
		proc.initialize()
		d.rp.add(proc)
	}

	var err error

	if d.firstPlaylist.ServerControl != nil &&
		d.firstPlaylist.ServerControl.CanBlockReload &&
		d.firstPlaylist.PreloadHint != nil {
		err = d.runLowLatency(ctx)
	} else {
		err = d.runTraditional(ctx)
	}

	d.segmentQueue.push(&segmentData{
		err: err,
	})
	<-ctx.Done()

	return fmt.Errorf("terminated")
}

func (d *clientStreamDownloader) runLowLatency(ctx context.Context) error {
	pl := d.firstPlaylist

	for {
		byts, err := d.downloadPreloadHint(ctx, pl.PreloadHint)
		if err != nil {
			return err
		}

		d.segmentQueue.push(&segmentData{
			dateTime: dateTimeOfPreloadHint(pl),
			payload:  byts,
		})

		pl, err = d.downloadPlaylist(ctx, d.firstPlaylist.ServerControl.CanSkipUntil != nil)
		if err != nil {
			return err
		}

		if pl.PreloadHint == nil {
			return fmt.Errorf("preload hint disappeared")
		}
	}
}

func (d *clientStreamDownloader) runTraditional(ctx context.Context) error {
	pl := d.firstPlaylist

	for {
		seg, payload, err := d.downloadNextSegment(ctx, pl)
		if err != nil {
			return err
		}

		d.segmentQueue.push(&segmentData{
			dateTime: seg.DateTime,
			payload:  payload,
		})

		ok := d.segmentQueue.waitUntilSizeIsBelow(ctx, 1)
		if !ok {
			return fmt.Errorf("terminated")
		}

		pl, err = d.downloadPlaylist(ctx, false)
		if err != nil {
			return err
		}
	}
}

func (d *clientStreamDownloader) downloadPlaylist(
	ctx context.Context,
	skipUntil bool,
) (*playlist.Media, error) {
	ur := d.playlistURL

	if skipUntil {
		newUR := cloneURL(ur)
		q := newUR.Query()
		q.Add("_HLS_skip", "YES")
		newUR.RawQuery = q.Encode()
		ur = newUR
	}

	d.onDownloadStreamPlaylist(ur.String())

	pl, err := downloadPlaylist(ctx, d.httpClient, d.onRequest, ur)
	if err != nil {
		return nil, err
	}

	plt, ok := pl.(*playlist.Media)
	if !ok {
		return nil, fmt.Errorf("invalid playlist")
	}

	return plt, nil
}

func (d *clientStreamDownloader) downloadPreloadHint(
	ctx context.Context,
	preloadHint *playlist.MediaPreloadHint,
) ([]byte, error) {
	u, err := clientAbsoluteURL(d.playlistURL, preloadHint.URI)
	if err != nil {
		return nil, err
	}

	d.onDownloadPart(u.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	if preloadHint.ByteRangeLength != nil {
		req.Header.Add("Range", "bytes="+strconv.FormatUint(preloadHint.ByteRangeStart, 10)+
			"-"+strconv.FormatUint(preloadHint.ByteRangeStart+*preloadHint.ByteRangeLength-1, 10))
	}

	d.onRequest(req)

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

func (d *clientStreamDownloader) downloadSegment(
	ctx context.Context,
	uri string,
	start *uint64,
	length *uint64,
) ([]byte, error) {
	u, err := clientAbsoluteURL(d.playlistURL, uri)
	if err != nil {
		return nil, err
	}

	d.onDownloadSegment(u.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	if length != nil {
		if start == nil {
			start = ptrOf(uint64(0))
		}
		req.Header.Add("Range", "bytes="+strconv.FormatUint(*start, 10)+
			"-"+strconv.FormatUint(*start+*length-1, 10))
	}

	d.onRequest(req)

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

func (d *clientStreamDownloader) downloadNextSegment(
	ctx context.Context,
	pl *playlist.Media,
) (*playlist.MediaSegment, []byte, error) {
	var seg *playlist.MediaSegment
	var segPos int

	if d.curSegmentID == nil {
		if (d.firstPlaylist.PlaylistType != nil &&
			*d.firstPlaylist.PlaylistType == playlist.MediaPlaylistTypeVOD) || d.firstPlaylist.Endlist {
			// VOD stream: start from the beginning
			if len(pl.Segments) == 0 {
				return nil, nil, fmt.Errorf("no segments found")
			}
			seg = pl.Segments[0]
		} else {
			// live stream: start from clientLiveInitialDistance
			seg, segPos = findSegmentWithInvPosition(pl.Segments, d.startDistance)
			if seg == nil {
				return nil, nil, fmt.Errorf("there aren't enough segments to fill the buffer")
			}
		}
	} else {
		var invPos int
		seg, segPos, invPos = findSegmentWithID(pl.MediaSequence, pl.Segments, *d.curSegmentID+1)
		if seg == nil {
			if pl.Endlist {
				return nil, nil, ErrClientEOS
			}
			return nil, nil, fmt.Errorf("next segment not found or not ready yet")
		}

		if !pl.Endlist && invPos > d.maxDistance {
			return nil, nil, fmt.Errorf("playback is too late")
		}
	}

	d.curSegmentID = ptrOf(pl.MediaSequence + segPos)

	byts, err := d.downloadSegment(ctx, seg.URI, seg.ByteRangeStart, seg.ByteRangeLength)
	if err != nil {
		return nil, nil, err
	}

	return seg, byts, nil
}

func (d *clientStreamDownloader) setTracks(ctx context.Context, tracks []*Track) ([]*clientTrack, bool) {
	select {
	case d.chTracks <- tracks:
	case <-ctx.Done():
		return nil, false
	}

	var allTracks map[*Track]*clientTrack

	select {
	case allTracks = <-d.chStartStreaming:
	case <-ctx.Done():
		return nil, false
	}

	streamTracks := make([]*clientTrack, len(tracks))
	for i, track := range tracks {
		streamTracks[i] = allTracks[track]
	}

	return streamTracks, true
}

func (d *clientStreamDownloader) onProcessorError(ctx context.Context, err error) {
	select {
	case d.chProcessorError <- err:
	case <-ctx.Done():
	}
}
