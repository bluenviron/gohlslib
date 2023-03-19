package storage

import (
	"fmt"
	"io"
	"os"
)

type segmentDisk struct {
	fpath     string
	f         *os.File
	parts     []*partDisk
	finalSize uint64
}

func newSegmentDisk(fpath string) (Segment, error) {
	f, err := os.Create(fpath)
	if err != nil {
		return nil, err
	}

	return &segmentDisk{
		fpath: fpath,
		f:     f,
	}, nil
}

// Finalize implements Segment.
func (s *segmentDisk) Finalize() {
	if len(s.parts) > 0 {
		// set size of last part
		lastPart := s.parts[len(s.parts)-1]
		lastPart.size = uint64(len(lastPart.buffer.Bytes()))

		// save size
		s.finalSize = lastPart.offset + lastPart.size
	}

	// remove segment from memory; we will use the file from now on
	for _, p := range s.parts {
		p.buffer = nil
	}

	s.f.Close()
	s.f = nil
}

// Remove implements Segment.
func (s *segmentDisk) Remove() {
	os.Remove(s.fpath)
}

// NewPart implements Segment.
func (s *segmentDisk) NewPart() Part {
	// set size of last part and get offset
	offset := uint64(0)
	if len(s.parts) > 0 {
		lastPart := s.parts[len(s.parts)-1]
		lastPart.size = uint64(len(lastPart.buffer.Bytes()))
		offset = lastPart.offset + lastPart.size
	}

	p := newPartDisk(s, offset)
	s.parts = append(s.parts, p)
	return p
}

// Reader implements Segment.
func (s *segmentDisk) Reader() (io.ReadCloser, error) {
	if s.f != nil {
		return nil, fmt.Errorf("segment has not been finalized yet")
	}

	return os.Open(s.fpath)
}

// Size implements Segment.
func (s *segmentDisk) Size() uint64 {
	return s.finalSize
}
