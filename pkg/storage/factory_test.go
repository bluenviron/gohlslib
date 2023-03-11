package storage

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStorage(t *testing.T) {
	for _, ca := range []string{
		"ram",
		"disk",
	} {
		t.Run(ca, func(t *testing.T) {
			var s Factory
			var dir string
			if ca == "ram" {
				s = NewFactoryRAM()
			} else {
				var err error
				dir, err = os.MkdirTemp("", "gohlslib")
				require.NoError(t, err)
				defer os.RemoveAll(dir)

				s = NewFactoryDisk(dir)
			}

			seg, err := s.NewSegment("myseg.mp4")
			require.NoError(t, err)

			part1 := seg.NewPart()

			w := part1.Writer()
			_, err = w.Write([]byte{1, 2, 3, 4})
			require.NoError(t, err)

			part2 := seg.NewPart()

			w = part2.Writer()
			_, err = w.Write([]byte{5, 6, 7, 8})
			require.NoError(t, err)

			r1, err := part1.Reader()
			require.NoError(t, err)

			buf, err := io.ReadAll(r1)
			require.NoError(t, err)
			require.Equal(t, []byte{1, 2, 3, 4}, buf)

			r1.Close()

			_, err = seg.Reader()
			require.EqualError(t, err, "segment has not been finalized yet")

			seg.Finalize()

			r1, err = part1.Reader()
			require.NoError(t, err)

			buf, err = io.ReadAll(r1)
			require.NoError(t, err)
			require.Equal(t, []byte{1, 2, 3, 4}, buf)

			r1.Close()

			r2, err := part2.Reader()
			require.NoError(t, err)

			buf, err = io.ReadAll(r2)
			require.NoError(t, err)
			require.Equal(t, []byte{5, 6, 7, 8}, buf)

			r2.Close()

			r, err := seg.Reader()
			require.NoError(t, err)

			buf, err = io.ReadAll(r)
			require.NoError(t, err)
			require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, buf)

			r.Close()

			if ca == "disk" {
				buf, err = os.ReadFile(filepath.Join(dir, "myseg.mp4"))
				require.NoError(t, err)
				require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, buf)
			}

			seg.Remove()

			if ca == "disk" {
				_, err = os.ReadFile(filepath.Join(dir, "myseg.mp4"))
				require.Error(t, err)
			}
		})
	}
}
