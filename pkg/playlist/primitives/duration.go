package primitives

import (
	"strconv"
	"time"
)

// Duration is a playlist duration.
type Duration time.Duration

// Unmarshal decodes a duration.
func (d *Duration) Unmarshal(val string) error {
	tmp, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return err
	}

	*d = Duration(time.Duration(tmp * float64(time.Second)))
	return nil
}
