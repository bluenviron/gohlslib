package gohlslib

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist"
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

func cloneURL(ur *url.URL) *url.URL {
	return &url.URL{
		Scheme:      ur.Scheme,
		Opaque:      ur.Opaque,
		User:        ur.User,
		Host:        ur.Host,
		Path:        ur.Path,
		RawPath:     ur.RawPath,
		OmitHost:    ur.OmitHost,
		ForceQuery:  ur.ForceQuery,
		RawQuery:    ur.RawQuery,
		Fragment:    ur.Fragment,
		RawFragment: ur.RawFragment,
	}
}

func downloadPlaylist(
	ctx context.Context,
	httpClient *http.Client,
	onRequest ClientOnRequestFunc,
	ur *url.URL,
) (playlist.Playlist, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ur.String(), nil)
	if err != nil {
		return nil, err
	}

	onRequest(req)

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

func getRenditionsByGroup(
	renditions []*playlist.MultivariantRendition,
	groupID string,
) []*playlist.MultivariantRendition {
	var ret []*playlist.MultivariantRendition

	for _, alt := range renditions {
		if alt.GroupID == groupID {
			ret = append(ret, alt)
		}
	}

	return ret
}

type clientPrimaryDownloaderClient interface {
	setTracks([]*Track) (map[*Track]*clientTrack, error)
	setLeadingTimeConv(ts clientTimeConv)
	waitLeadingTimeConv(ctx context.Context) bool
	getLeadingTimeConv() clientTimeConv
}

type clientPrimaryDownloader struct {
	primaryPlaylistURL        *url.URL
	startDistance             int
	maxDistance               int
	httpClient                *http.Client
	rp                        *clientRoutinePool
	onRequest                 ClientOnRequestFunc
	onDownloadPrimaryPlaylist ClientOnDownloadPrimaryPlaylistFunc
	onDownloadStreamPlaylist  ClientOnDownloadStreamPlaylistFunc
	onDownloadSegment         ClientOnDownloadSegmentFunc
	onDownloadPart            ClientOnDownloadPartFunc
	onDecodeError             ClientOnDecodeErrorFunc
	client                    clientPrimaryDownloaderClient

	clientTracks map[*Track]*clientTrack
}

func (d *clientPrimaryDownloader) initialize() {
}

func (d *clientPrimaryDownloader) run(ctx context.Context) error {
	d.onDownloadPrimaryPlaylist(d.primaryPlaylistURL.String())

	pl, err := downloadPlaylist(ctx, d.httpClient, d.onRequest, d.primaryPlaylistURL)
	if err != nil {
		return err
	}

	var streams []*clientStreamDownloader

	switch plt := pl.(type) {
	case *playlist.Media:
		stream := &clientStreamDownloader{
			isLeading:                true,
			startDistance:            d.startDistance,
			maxDistance:              d.maxDistance,
			httpClient:               d.httpClient,
			onRequest:                d.onRequest,
			onDownloadStreamPlaylist: d.onDownloadStreamPlaylist,
			onDownloadSegment:        d.onDownloadSegment,
			onDownloadPart:           d.onDownloadPart,
			onDecodeError:            d.onDecodeError,
			playlistURL:              d.primaryPlaylistURL,
			firstPlaylist:            plt,
			rp:                       d.rp,
			client:                   d.client,
		}
		stream.initialize()
		d.rp.add(stream)
		streams = append(streams, stream)

	case *playlist.Multivariant:
		leadingPlaylist := pickLeadingPlaylist(plt.Variants)
		if leadingPlaylist == nil {
			return fmt.Errorf("no variants with supported codecs found")
		}

		var u *url.URL
		u, err = clientAbsoluteURL(d.primaryPlaylistURL, leadingPlaylist.URI)
		if err != nil {
			return err
		}

		stream := &clientStreamDownloader{
			isLeading:                true,
			startDistance:            d.startDistance,
			maxDistance:              d.maxDistance,
			httpClient:               d.httpClient,
			onRequest:                d.onRequest,
			onDownloadStreamPlaylist: d.onDownloadStreamPlaylist,
			onDownloadSegment:        d.onDownloadSegment,
			onDownloadPart:           d.onDownloadPart,
			onDecodeError:            d.onDecodeError,
			playlistURL:              u,
			firstPlaylist:            nil,
			rp:                       d.rp,
			client:                   d.client,
		}
		stream.initialize()
		d.rp.add(stream)
		streams = append(streams, stream)

		if leadingPlaylist.Audio != "" {
			audioPlaylists := getRenditionsByGroup(plt.Renditions, leadingPlaylist.Audio)
			if audioPlaylists == nil {
				return fmt.Errorf("no playlist with Group ID \"%s\" found", leadingPlaylist.Audio)
			}

			for _, pl := range audioPlaylists {
				// stream data already included in the leading playlist
				if pl.URI == nil {
					continue
				}

				if pl.URI != nil {
					u, err = clientAbsoluteURL(d.primaryPlaylistURL, *pl.URI)
					if err != nil {
						return err
					}

					stream = &clientStreamDownloader{
						isLeading:                false,
						onRequest:                d.onRequest,
						startDistance:            d.startDistance,
						maxDistance:              d.maxDistance,
						httpClient:               d.httpClient,
						onDownloadStreamPlaylist: d.onDownloadStreamPlaylist,
						onDownloadSegment:        d.onDownloadSegment,
						onDownloadPart:           d.onDownloadPart,
						onDecodeError:            d.onDecodeError,
						playlistURL:              u,
						rendition:                pl,
						rp:                       d.rp,
						client:                   d.client,
					}
					stream.initialize()
					d.rp.add(stream)
					streams = append(streams, stream)
				}
			}
		}

	default:
		return fmt.Errorf("invalid playlist")
	}

	var tracks []*Track

	for _, stream := range streams {
		select {
		case streamTracks := <-stream.chTracks:
			tracks = append(tracks, streamTracks...)

		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	if len(tracks) == 0 {
		return fmt.Errorf("no supported tracks found")
	}

	d.clientTracks, err = d.client.setTracks(tracks)
	if err != nil {
		return err
	}

	for _, stream := range streams {
		select {
		case stream.chStartStreaming <- d.clientTracks:
		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	for _, stream := range streams {
		select {
		case err = <-stream.chProcessorError:
		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	return err
}
