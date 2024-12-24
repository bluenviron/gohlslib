package playlist

import (
	"fmt"
	"strconv"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist/primitives"
)

// MediaPartInf is a EXT-X-PART-INF tag.
type MediaPartInf struct {
	// PART-TARGET
	// required
	PartTarget time.Duration
}

func (t *MediaPartInf) unmarshal(v string) error {
	var attrs primitives.Attributes
	err := attrs.Unmarshal(v)
	if err != nil {
		return err
	}

	for key, val := range attrs {
		if key == "PART-TARGET" {
			var d primitives.Duration
			err := d.Unmarshal(val)
			if err != nil {
				return err
			}
			t.PartTarget = time.Duration(d)
		}
	}

	if t.PartTarget == 0 {
		return fmt.Errorf("PART-TARGET missing")
	}

	return nil
}

func (t MediaPartInf) marshal() string {
	return "#EXT-X-PART-INF:PART-TARGET=" + strconv.FormatFloat(t.PartTarget.Seconds(), 'f', 5, 64) + "\n"
}
