package primitives

import (
	"fmt"
)

// SkipHeader reads and skips a playlist header.
func SkipHeader(s string) (string, error) {
	var line string
	line, s = ReadLine(s)

	if line != "#EXTM3U" {
		return "", fmt.Errorf("M3U8 header is missing")
	}
	return s, nil
}
