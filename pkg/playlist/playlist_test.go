package playlist_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist"
)

func TestUnmarshal(t *testing.T) {
	for _, ca := range casesMultivariant {
		t.Run("multivariant_"+ca.name, func(t *testing.T) {
			dec, err := playlist.Unmarshal([]byte(ca.input))
			require.NoError(t, err)
			require.Equal(t, &ca.dec, dec)
		})
	}

	for _, ca := range casesMedia {
		t.Run("media_"+ca.name, func(t *testing.T) {
			dec, err := playlist.Unmarshal([]byte(ca.input))
			require.NoError(t, err)
			require.Equal(t, &ca.dec, dec)
		})
	}
}

func FuzzPlaylistUnmarshal(f *testing.F) {
	for _, ca := range casesMultivariant {
		f.Add(ca.input)
	}

	for _, ca := range casesMedia {
		f.Add(ca.input)
	}

	f.Fuzz(func(t *testing.T, a string) {
		pl, err := playlist.Unmarshal([]byte(a))
		if err != nil {
			return
		}

		_, err = pl.Marshal()
		require.NoError(t, err)
	})
}
