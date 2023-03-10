package mpegts

import (
	"fmt"
)

// OpusAccessUnit is a MPEG-TS Opus access unit.
type OpusAccessUnit struct {
	ControlHeader OpusControlHeader
	Frame         []byte
}

// Unmarshal decodes an access unit.
func (au *OpusAccessUnit) Unmarshal(buf []byte) (int, error) {
	n, err := au.ControlHeader.Unmarshal(buf)
	if err != nil {
		return 0, fmt.Errorf("could not decode Opus control header: %v", err)
	}
	buf = buf[n:]

	if len(buf) < int(au.ControlHeader.PayloadSize) {
		return 0, fmt.Errorf("buffer is too small")
	}

	au.Frame = buf[:au.ControlHeader.PayloadSize]

	return n + int(au.ControlHeader.PayloadSize), nil
}
