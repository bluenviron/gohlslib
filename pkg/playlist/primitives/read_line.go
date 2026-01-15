package primitives

import (
	"strings"
)

// ReadLine reads a line from a string. It returns the line and the remaining string.
func ReadLine(s string) (string, string) {
	line, remaining, found := strings.Cut(s, "\n")
	if !found {
		return s, ""
	}

	if len(line) != 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}

	return line, remaining
}
