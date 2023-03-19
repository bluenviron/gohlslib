package storage

import (
	"bytes"
	"io"

	"github.com/orcaman/writerseeker"
)

type partDisk struct {
	s      *segmentDisk
	buffer *writerseeker.WriterSeeker
	offset uint64
	size   uint64
}

func newPartDisk(s *segmentDisk, offset uint64) *partDisk {
	return &partDisk{
		s:      s,
		buffer: &writerseeker.WriterSeeker{},
		offset: offset,
	}
}

// Writer implements Part.
func (p *partDisk) Writer() io.WriteSeeker {
	// write on both disk and RAM
	return newDoubleWriter(newOffsetWriter(p.s.f, int64(p.offset)), p.buffer)
}

// Reader implements Part.
func (p *partDisk) Reader() (io.ReadCloser, error) {
	// read from RAM if possible
	if p.buffer != nil {
		return io.NopCloser(bytes.NewReader(p.buffer.Bytes())), nil
	}

	// read from disk
	return newDiskPartReader(p.s.fpath, p.offset, p.size)
}
