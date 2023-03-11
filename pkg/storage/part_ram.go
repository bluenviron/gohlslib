package storage

import (
	"bytes"
	"io"

	"github.com/orcaman/writerseeker"
)

type partRAM struct {
	buffer *writerseeker.WriterSeeker
}

func newPartRAM() *partRAM {
	return &partRAM{
		buffer: &writerseeker.WriterSeeker{},
	}
}

// Writer implements Part.
func (p *partRAM) Writer() io.WriteSeeker {
	return p.buffer
}

// Reader implements Part.
func (p *partRAM) Reader() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(p.buffer.Bytes())), nil
}
