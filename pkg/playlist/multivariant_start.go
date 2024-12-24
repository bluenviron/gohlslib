package playlist

import (
	"fmt"
	"strconv"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist/primitives"
)

// MultivariantStart is a EXT-X-START tag.
type MultivariantStart struct {
	// TIME-OFFSET
	// required
	TimeOffset time.Duration
}

func (t *MultivariantStart) unmarshal(v string) error {
	var attrs primitives.Attributes
	err := attrs.Unmarshal(v)
	if err != nil {
		return err
	}

	for key, val := range attrs {
		if key == "TIME-OFFSET" {
			var d primitives.Duration
			err := d.Unmarshal(val)
			if err != nil {
				return err
			}
			t.TimeOffset = time.Duration(d)
		}
	}

	if t.TimeOffset == 0 {
		return fmt.Errorf("TIME-OFFSET missing")
	}

	return nil
}

func (t MultivariantStart) marshal() string {
	return "#EXT-X-START:TIME-OFFSET=" + strconv.FormatFloat(t.TimeOffset.Seconds(), 'f', 5, 64) + "\n"
}
