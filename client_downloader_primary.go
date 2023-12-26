package gohlslib

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/bluenviron/gohlslib/pkg/playlist"
)

func checkSupport(codecs []string) bool {
	for _, codec := range codecs {
		if !strings.HasPrefix(codec, "avc1.") &&
			!strings.HasPrefix(codec, "hvc1.") &&
			!strings.HasPrefix(codec, "hev1.") &&
			!strings.HasPrefix(codec, "mp4a.") &&
			codec != "opus" {
			return false
		}
	}
	return true
}

func clientDownloadPlaylist(ctx context.Context, httpClient *http.Client, ur *url.URL) (playlist.Playlist, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ur.String(), nil)
	if err != nil {
		return nil, err
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", res.StatusCode)
	}

	byts, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return playlist.Unmarshal(byts)
}

func pickLeadingPlaylist(variants []*playlist.MultivariantVariant) *playlist.MultivariantVariant {
	var candidates []*playlist.MultivariantVariant //nolint:prealloc
	for _, v := range variants {
		if !checkSupport(v.Codecs) {
			continue
		}
		candidates = append(candidates, v)
	}
	if candidates == nil {
		return nil
	}

	// pick the variant with the greatest bandwidth
	var leadingPlaylist *playlist.MultivariantVariant
	for _, v := range candidates {
		if leadingPlaylist == nil ||
			v.Bandwidth > leadingPlaylist.Bandwidth {
			leadingPlaylist = v
		}
	}
	return leadingPlaylist
}

func pickAudioPlaylist(alternatives []*playlist.MultivariantRendition, groupID string) *playlist.MultivariantRendition {
	candidates := func() []*playlist.MultivariantRendition {
		var ret []*playlist.MultivariantRendition
		for _, alt := range alternatives {
			if alt.GroupID == groupID {
				ret = append(ret, alt)
			}
		}
		return ret
	}()
	if candidates == nil {
		return nil
	}

	// pick the default audio playlist
	for _, alt := range candidates {
		if alt.Default {
			return alt
		}
	}

	// alternatively, pick the first one
	return candidates[0]
}

type clientTimeSync interface{}

type clientDownloaderPrimary struct {
	primaryPlaylistURL        *url.URL
	httpClient                *http.Client
	onDownloadPrimaryPlaylist ClientOnDownloadPrimaryPlaylistFunc
	onDownloadStreamPlaylist  ClientOnDownloadStreamPlaylistFunc
	onDownloadSegment         ClientOnDownloadSegmentFunc
	onDecodeError             ClientOnDecodeErrorFunc
	rp                        *clientRoutinePool
	onTracks                  ClientOnTracksFunc
	onData                    map[*Track]interface{}

	leadingTimeSync clientTimeSync

	// in
	streamTracks chan []*Track

	// out
	startStreaming       chan struct{}
	leadingTimeSyncReady chan struct{}
}

func (d *clientDownloaderPrimary) initialize() {
	d.streamTracks = make(chan []*Track)
	d.startStreaming = make(chan struct{})
	d.leadingTimeSyncReady = make(chan struct{})
}

func (d *clientDownloaderPrimary) run(ctx context.Context) error {
	d.onDownloadPrimaryPlaylist(d.primaryPlaylistURL.String())

	pl, err := clientDownloadPlaylist(ctx, d.httpClient, d.primaryPlaylistURL)
	if err != nil {
		return err
	}

	streamCount := 0

	switch plt := pl.(type) {
	case *playlist.Media:
		ds := &clientDownloaderStream{
			isLeading:                true,
			httpClient:               d.httpClient,
			onDownloadStreamPlaylist: d.onDownloadStreamPlaylist,
			onDownloadSegment:        d.onDownloadSegment,
			onDecodeError:            d.onDecodeError,
			playlistURL:              d.primaryPlaylistURL,
			initialPlaylist:          plt,
			rp:                       d.rp,
			onStreamTracks:           d.onStreamTracks,
			onSetLeadingTimeSync:     d.onSetLeadingTimeSync,
			onGetLeadingTimeSync:     d.onGetLeadingTimeSync,
			onData:                   d.onData,
		}
		d.rp.add(ds)
		streamCount++

	case *playlist.Multivariant:
		leadingPlaylist := pickLeadingPlaylist(plt.Variants)
		if leadingPlaylist == nil {
			return fmt.Errorf("no variants with supported codecs found")
		}

		u, err := clientAbsoluteURL(d.primaryPlaylistURL, leadingPlaylist.URI)
		if err != nil {
			return err
		}

		ds := &clientDownloaderStream{
			isLeading:                true,
			httpClient:               d.httpClient,
			onDownloadStreamPlaylist: d.onDownloadStreamPlaylist,
			onDownloadSegment:        d.onDownloadSegment,
			onDecodeError:            d.onDecodeError,
			playlistURL:              u,
			initialPlaylist:          nil,
			rp:                       d.rp,
			onStreamTracks:           d.onStreamTracks,
			onSetLeadingTimeSync:     d.onSetLeadingTimeSync,
			onGetLeadingTimeSync:     d.onGetLeadingTimeSync,
			onData:                   d.onData,
		}
		d.rp.add(ds)
		streamCount++

		if leadingPlaylist.Audio != "" {
			audioPlaylist := pickAudioPlaylist(plt.Renditions, leadingPlaylist.Audio)
			if audioPlaylist == nil {
				return fmt.Errorf("audio playlist with id \"%s\" not found", leadingPlaylist.Audio)
			}

			if audioPlaylist.URI != "" {
				u, err := clientAbsoluteURL(d.primaryPlaylistURL, audioPlaylist.URI)
				if err != nil {
					return err
				}

				ds := &clientDownloaderStream{
					isLeading:                false,
					httpClient:               d.httpClient,
					onDownloadStreamPlaylist: d.onDownloadStreamPlaylist,
					onDownloadSegment:        d.onDownloadSegment,
					onDecodeError:            d.onDecodeError,
					playlistURL:              u,
					initialPlaylist:          nil,
					rp:                       d.rp,
					onStreamTracks:           d.onStreamTracks,
					onSetLeadingTimeSync:     d.onSetLeadingTimeSync,
					onGetLeadingTimeSync:     d.onGetLeadingTimeSync,
					onData:                   d.onData,
				}
				d.rp.add(ds)
				streamCount++
			}
		}

	default:
		return fmt.Errorf("invalid playlist")
	}

	var tracks []*Track

	for i := 0; i < streamCount; i++ {
		select {
		case streamTracks := <-d.streamTracks:
			tracks = append(tracks, streamTracks...)
		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	if len(tracks) == 0 {
		return fmt.Errorf("no supported tracks found")
	}

	err = d.onTracks(tracks)
	if err != nil {
		return err
	}

	close(d.startStreaming)

	return nil
}

func (d *clientDownloaderPrimary) onStreamTracks(ctx context.Context, tracks []*Track) bool {
	select {
	case d.streamTracks <- tracks:
	case <-ctx.Done():
		return false
	}

	select {
	case <-d.startStreaming:
	case <-ctx.Done():
		return false
	}

	return true
}

func (d *clientDownloaderPrimary) onSetLeadingTimeSync(ts clientTimeSync) {
	d.leadingTimeSync = ts
	close(d.leadingTimeSyncReady)
}

func (d *clientDownloaderPrimary) onGetLeadingTimeSync(ctx context.Context) (clientTimeSync, bool) {
	select {
	case <-d.leadingTimeSyncReady:
	case <-ctx.Done():
		return nil, false
	}
	return d.leadingTimeSync, true
}
