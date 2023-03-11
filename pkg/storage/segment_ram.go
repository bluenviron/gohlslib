package storage

import (
	"fmt"
	"io"
)

type segmentRAM struct {
	finalized bool
	parts     []*partRAM
}

func newSegmentRAM() Segment {
	return &segmentRAM{}
}

// Finalize implements Segment.
func (s *segmentRAM) Finalize() {
	s.finalized = true
}

// Remove implements Segment.
func (s *segmentRAM) Remove() {
}

// NewPart implements Segment.
func (s *segmentRAM) NewPart() Part {
	p := newPartRAM()
	s.parts = append(s.parts, p)
	return p
}

// Reader implements Segment.
func (s *segmentRAM) Reader() (io.ReadCloser, error) {
	if !s.finalized {
		return nil, fmt.Errorf("segment has not been finalized yet")
	}

	return io.NopCloser(&ramSegmentReader{
		parts: s.parts,
	}), nil
}
