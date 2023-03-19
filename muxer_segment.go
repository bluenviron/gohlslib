package gohlslib

import (
	"fmt"
	"io"
	"time"
)

type muxerSegment interface {
	close()
	getName() string
	getDuration() time.Duration
	getSize() uint64
	reader() (io.ReadCloser, error)
}

type muxerGap struct {
	duration time.Duration
}

func (g muxerGap) close() {
}

func (g muxerGap) getName() string {
	return ""
}

func (g muxerGap) getDuration() time.Duration {
	return g.duration
}

func (g muxerGap) getSize() uint64 {
	return 0
}

func (g muxerGap) reader() (io.ReadCloser, error) {
	return nil, fmt.Errorf("unimplemented")
}
