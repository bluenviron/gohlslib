package primitives

import (
	"bufio"
	"fmt"
)

// HeaderUnmarshal decodes a header.
func HeaderUnmarshal(r *bufio.Reader) error {
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	line = RemoveReturn(line)

	if line != "#EXTM3U" {
		return fmt.Errorf("M3U8 header is missing")
	}

	return nil
}
