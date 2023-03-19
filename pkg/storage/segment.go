package storage

import (
	"io"
)

// Segment is the underlying storage of a HLS segment.
type Segment interface {
	// Finalize finalizes the segment, making it read-only.
	// It must always be called to avoid a memory leak.
	Finalize()

	// Remove removes the segment from disk.
	Remove()

	// NewPart creates a new part storage.
	NewPart() Part

	// Reader returns a ReadCloser to read the segment.
	// Close() must always be called to avoid a memory leak.
	Reader() (io.ReadCloser, error)

	// Size returns the size of the segment.
	Size() uint64
}
