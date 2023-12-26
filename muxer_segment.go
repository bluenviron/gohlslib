package gohlslib

import (
	"io"
	"strconv"
	"time"

	"github.com/bluenviron/gohlslib/pkg/playlist"
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
	getDuration() time.Duration
	getSize() uint64
	isForceSwitched() bool
	definition(showDateAndParts bool) *playlist.MediaSegment
	reader() (io.ReadCloser, error)
}
