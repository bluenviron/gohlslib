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
	reader() (io.ReadCloser, error)
}

type muxerGap struct {
	renderedDuration time.Duration
}

func (g muxerGap) close() {
}

func (g muxerGap) getName() string {
	return ""
}

func (g muxerGap) getDuration() time.Duration {
	return g.renderedDuration
}

func (g muxerGap) reader() (io.ReadCloser, error) {
	return nil, fmt.Errorf("unimplemented")
}
