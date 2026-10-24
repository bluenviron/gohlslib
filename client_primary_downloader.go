package gohlslib

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/sync/errgroup"

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
	errGroupCtx context.Context,
	httpClient *http.Client,
	onRequest ClientOnRequestFunc,
	ur *url.URL,
) (*url.URL, playlist.Playlist, error) {
	req, err := http.NewRequestWithContext(errGroupCtx, http.MethodGet, ur.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	onRequest(req)

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("bad status code: %d", res.StatusCode)
	}

	byts, err := io.ReadAll(&customLimitReader{res.Body, clientMaxInboundPlaylistSize})
	if err != nil {
		return nil, nil, err
	}

	pl, err := playlist.Unmarshal(byts)
	if err != nil {
		return nil, nil, err
	}

	return res.Request.URL, pl, nil
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
	setTimeConv(ts clientTimeConv)
	waitTimeConv(errGroupCtx context.Context) (clientTimeConv, bool)
}

type clientPrimaryDownloader struct {
	primaryPlaylistURL        *url.URL
	startDistance             int
	maxDistance               int
	httpClient                *http.Client
	errGroup                  *errgroup.Group
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

func (d *clientPrimaryDownloader) run(errGroupCtx context.Context) error {
	d.onDownloadPrimaryPlaylist(d.primaryPlaylistURL.String())

	finalURL, pl, err := downloadPlaylist(errGroupCtx, d.httpClient, d.onRequest, d.primaryPlaylistURL)
	if err != nil {
		return err
	}

	d.primaryPlaylistURL = finalURL

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
			playlistURL:              finalURL,
			firstPlaylist:            plt,
			errGroup:                 d.errGroup,
			client:                   d.client,
		}
		stream.initialize()

		d.errGroup.Go(func() error {
			return stream.run(errGroupCtx)
		})

		streams = append(streams, stream)

	case *playlist.Multivariant:
		leadingPlaylist := pickLeadingPlaylist(plt.Variants)
		if leadingPlaylist == nil {
			return fmt.Errorf("no variants with supported codecs found")
		}

		var u *url.URL
		u, err = clientAbsoluteURL(finalURL, leadingPlaylist.URI)
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
			errGroup:                 d.errGroup,
			client:                   d.client,
		}
		stream.initialize()

		d.errGroup.Go(func() error {
			return stream.run(errGroupCtx)
		})

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

					renditionStream := &clientStreamDownloader{
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
						errGroup:                 d.errGroup,
						client:                   d.client,
					}
					renditionStream.initialize()

					d.errGroup.Go(func() error {
						return renditionStream.run(errGroupCtx)
					})

					streams = append(streams, renditionStream)
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

		case <-errGroupCtx.Done():
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
		case <-errGroupCtx.Done():
			return fmt.Errorf("terminated")
		}
	}

	for _, stream := range streams {
		select {
		case err = <-stream.chProcessorError:
		case <-errGroupCtx.Done():
			return fmt.Errorf("terminated")
		}
	}

	return err
}
