package primitives

import (
	"strconv"
	"strings"
)

// ByteRange is a byte range.
type ByteRange struct {
	Length uint64
	Start  *uint64
}

// Unmarshal decodes a byte range.
func (b *ByteRange) Unmarshal(v string) error {
	if str1, str2, found := strings.Cut(v, "@"); found {
		var err error
		b.Length, err = strconv.ParseUint(str1, 10, 64)
		if err != nil {
			return err
		}

		start, err := strconv.ParseUint(str2, 10, 64)
		if err != nil {
			return err
		}

		b.Start = &start

		return nil
	}

	var err error
	b.Length, err = strconv.ParseUint(v, 10, 64)
	if err != nil {
		return err
	}

	return nil
}

// Marshal encodes a byte range.
func (b ByteRange) Marshal() string {
	ret := strconv.FormatUint(b.Length, 10)

	if b.Start != nil {
		ret += "@" + strconv.FormatUint(*b.Start, 10)
	}

	return ret
}
