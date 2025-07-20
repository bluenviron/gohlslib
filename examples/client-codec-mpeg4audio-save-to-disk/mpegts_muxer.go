package main

import (
	"bufio"
	"os"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
)

// mpegtsMuxer allows to save a MPEG4-audio stream into a MPEG-TS file.
type mpegtsMuxer struct {
	fileName string
	config   *mpeg4audio.AudioSpecificConfig

	f     *os.File
	b     *bufio.Writer
	w     *mpegts.Writer
	track *mpegts.Track
}

// initialize initializes a mpegtsMuxer.
func (e *mpegtsMuxer) initialize() error {
	var err error
	e.f, err = os.Create(e.fileName)
	if err != nil {
		return err
	}
	e.b = bufio.NewWriter(e.f)

	e.track = &mpegts.Track{
		Codec: &mpegts.CodecMPEG4Audio{
			Config: *e.config,
		},
	}

	e.w = &mpegts.Writer{
		W:      e.b,
		Tracks: []*mpegts.Track{e.track},
	}
	err = e.w.Initialize()
	if err != nil {
		return err
	}

	return nil
}

// close closes all the mpegtsMuxer resources.
func (e *mpegtsMuxer) close() {
	e.b.Flush() //nolint:errcheck
	e.f.Close()
}

// writeMPEG4Audio writes MPEG-4 audio access units into MPEG-TS.
func (e *mpegtsMuxer) writeMPEG4Audio(aus [][]byte, pts int64) error {
	return e.w.WriteMPEG4Audio(e.track, pts, aus)
}
