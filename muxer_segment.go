package gohlslib

import (
	"fmt"
	"io"
	"strconv"
	"time"
)

func segmentName(prefix string, id uint64, mp4 bool) string {
	if mp4 {
		return prefix + "_seg" + strconv.FormatUint(id, 10) + ".mp4"
	}
	return prefix + "_seg" + strconv.FormatUint(id, 10) + ".ts"
}

type muxerSegment interface {
	close()
	getName() string
	hasDuration() bool
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

func (g muxerGap) hasDuration() bool {
	return true
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
