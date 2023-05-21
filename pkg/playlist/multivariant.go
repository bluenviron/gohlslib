package playlist

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/bluenviron/gohlslib/pkg/playlist/primitives"
)

// Multivariant is a multivariant playlist.
type Multivariant struct {
	// #EXT-X-VERSION
	// required
	Version int

	// #EXT-X-INDEPENDENT-SEGMENTS
	IndependentSegments bool

	// #EXT-X-START
	Start *MultivariantStart

	// #EXT-X-STREAM-INF
	// at least one is required
	Variants []*MultivariantVariant

	// #EXT-X-MEDIA
	Renditions []*MultivariantRendition
}

func (m Multivariant) isPlaylist() {}

// Unmarshal decodes the playlist.
func (m *Multivariant) Unmarshal(buf []byte) error {
	r := bufio.NewReader(bytes.NewReader(buf))

	err := primitives.HeaderUnmarshal(r)
	if err != nil {
		return err
	}

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		line = primitives.RemoveReturn(line)

		switch {
		case strings.HasPrefix(line, "#EXT-X-VERSION:"):
			line = line[len("#EXT-X-VERSION:"):]

			tmp, err := strconv.ParseUint(line, 10, 31)
			if err != nil {
				return err
			}
			m.Version = int(tmp)

			if m.Version > maxSupportedVersion {
				return fmt.Errorf("unsupported HLS version (%d)", m.Version)
			}

		case strings.HasPrefix(line, "#EXT-X-INDEPENDENT-SEGMENTS"):
			m.IndependentSegments = true

		case strings.HasPrefix(line, "#EXT-X-START:"):
			line = line[len("#EXT-X-START:"):]

			m.Start = &MultivariantStart{}
			err := m.Start.unmarshal(line)
			if err != nil {
				return err
			}

		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			line = line[len("#EXT-X-STREAM-INF:"):]

			line2, err := r.ReadString('\n')
			if err != nil {
				return err
			}
			line2 = primitives.RemoveReturn(line2)
			line += "\n" + line2

			var v MultivariantVariant
			err = v.unmarshal(line)
			if err != nil {
				return fmt.Errorf("invalid variant: %s", err)
			}

			m.Variants = append(m.Variants, &v)

		case strings.HasPrefix(line, "#EXT-X-MEDIA:"):
			line = line[len("#EXT-X-MEDIA:"):]

			var r MultivariantRendition
			err = r.unmarshal(line)
			if err != nil {
				return fmt.Errorf("invalid rendition: %s", err)
			}

			m.Renditions = append(m.Renditions, &r)
		}
	}

	if len(m.Variants) == 0 {
		return fmt.Errorf("no variants found")
	}

	return nil
}

// Marshal encodes the playlist.
func (m Multivariant) Marshal() ([]byte, error) {
	ret := "#EXTM3U\n" +
		"#EXT-X-VERSION:" + strconv.FormatInt(int64(m.Version), 10) + "\n"

	if m.IndependentSegments {
		ret += "#EXT-X-INDEPENDENT-SEGMENTS\n"
	}

	if m.Start != nil {
		ret += m.Start.marshal()
	}

	ret += "\n"

	for _, v := range m.Variants {
		ret += v.marshal()
	}

	if len(m.Renditions) != 0 {
		ret += "\n"

		for _, r := range m.Renditions {
			ret += r.marshal()
		}
	}

	return []byte(ret), nil
}
