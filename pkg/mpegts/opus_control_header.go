package mpegts

import (
	"fmt"

	"github.com/aler9/gortsplib/v2/pkg/bits"
)

// OpusControlHeader is a MPEG-TS Opus control header.
type OpusControlHeader struct {
	PayloadSize uint64
	StartTrim   uint16
	EndTrim     uint16
}

// Unmarshal decodes a control header.
func (h *OpusControlHeader) Unmarshal(buf []byte) (int, error) {
	pos := 0

	err := bits.HasSpace(buf, pos, 16)
	if err != nil {
		return 0, err
	}

	prefix := bits.ReadBitsUnsafe(buf, &pos, 11)
	if prefix != 0x3ff {
		return 0, fmt.Errorf("invalid prefix")
	}

	startTrimFlag := bits.ReadFlagUnsafe(buf, &pos)
	endTrimFlag := bits.ReadFlagUnsafe(buf, &pos)
	controlExtensionFlag := bits.ReadFlagUnsafe(buf, &pos)

	pos += 2 // reserved

	h.PayloadSize = 0

	for {
		next, err := bits.ReadBits(buf, &pos, 8)
		if err != nil {
			return 0, err
		}

		h.PayloadSize += next

		if next != 0xFF {
			break
		}
	}

	if startTrimFlag {
		err := bits.HasSpace(buf, pos, 16)
		if err != nil {
			return 0, err
		}

		pos += 3 // reserved
		h.StartTrim = uint16(bits.ReadBitsUnsafe(buf, &pos, 13))
	}

	if endTrimFlag {
		err := bits.HasSpace(buf, pos, 16)
		if err != nil {
			return 0, err
		}

		pos += 3 // reservedch.PayloadSize
		h.EndTrim = uint16(bits.ReadBitsUnsafe(buf, &pos, 13))
	}

	if controlExtensionFlag {
		le, err := bits.ReadBits(buf, &pos, 8)
		if err != nil {
			return 0, err
		}

		space := int(8 * le)
		err = bits.HasSpace(buf, pos, space)
		if err != nil {
			return 0, err
		}

		pos += space // reserved
	}

	return pos / 8, nil
}
