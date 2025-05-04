package playlist

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnmarshal(t *testing.T) {
	for _, ca := range casesMultivariant {
		t.Run("multivariant_"+ca.name, func(t *testing.T) {
			dec, err := Unmarshal([]byte(ca.input))
			require.NoError(t, err)
			require.Equal(t, dec, &ca.dec)
		})
	}

	for _, ca := range casesMedia {
		t.Run("media_"+ca.name, func(t *testing.T) {
			dec, err := Unmarshal([]byte(ca.input))
			require.NoError(t, err)
			require.Equal(t, dec, &ca.dec)
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
		pl, err := Unmarshal([]byte(a))
		if err != nil {
			return
		}

		_, err = pl.Marshal()
		require.NoError(t, err)
	})
}
