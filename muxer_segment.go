package gohlslib

import (
	"fmt"
	"io"
	"time"
)

type muxerSegment interface {
	close()
	finalize(time.Duration) error
	getPath() string
	getDuration() time.Duration
	getSize() uint64
	isFromForcedRotation() bool
	reader() (io.ReadCloser, error)
}

type muxerGap struct {
	duration time.Duration
}

func (muxerGap) close() {
}

func (muxerGap) finalize(time.Duration) error {
	return nil
}

func (muxerGap) getPath() string {
	return ""
}

func (g muxerGap) getDuration() time.Duration {
	return g.duration
}

func (muxerGap) getSize() uint64 {
	return 0
}

func (muxerGap) isFromForcedRotation() bool {
	return false
}

func (muxerGap) reader() (io.ReadCloser, error) {
	return nil, fmt.Errorf("unimplemented")
}
