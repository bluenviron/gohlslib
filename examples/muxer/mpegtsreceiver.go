package main

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/asticode/go-astits"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
)

type packetConnReader struct {
	pc        net.PacketConn
	midbuf    []byte
	midbufpos int
}

func newPacketConnReader(pc net.PacketConn) *packetConnReader {
	return &packetConnReader{
		pc:     pc,
		midbuf: make([]byte, 0, 1500),
	}
}

func (r *packetConnReader) Read(p []byte) (int, error) {
	if r.midbufpos < len(r.midbuf) {
		n := copy(p, r.midbuf[r.midbufpos:])
		r.midbufpos += n
		return n, nil
	}

	mn, _, err := r.pc.ReadFrom(r.midbuf[:cap(r.midbuf)])
	if err != nil {
		return 0, err
	}

	if (mn % 188) != 0 {
		return 0, fmt.Errorf("received packet with size %d not multiple of 188", mn)
	}

	r.midbuf = r.midbuf[:mn]
	n := copy(p, r.midbuf)
	r.midbufpos = n
	return n, nil
}

// mpegtsReceiver is a utility to receive MPEG-TS/H264 packets.
type mpegtsReceiver struct {
	pc      net.PacketConn
	dem     *astits.Demuxer
	timedec *mpegts.TimeDecoder
}

// newMPEGTSReceiver allocates a mpegtsReceiver.
func newMPEGTSReceiver() (*mpegtsReceiver, error) {
	pc, err := net.ListenPacket("udp", "localhost:9000")
	if err != nil {
		return nil, err
	}

	// allocate MPEG-TS demuxer
	dem := astits.NewDemuxer(context.Background(), newPacketConnReader(pc), astits.DemuxerOptPacketSize(188))

	return &mpegtsReceiver{
		pc:  pc,
		dem: dem,
	}, nil
}

func (r *mpegtsReceiver) Close() {
	r.pc.Close()
}

// Read reads a H264 access unit.
func (r *mpegtsReceiver) Read() ([][]byte, time.Duration, error) {
	for {
		// read data
		data, err := r.dem.NextData()
		if err != nil {
			return nil, 0, err
		}

		// wait for a PES
		if data.PES == nil {
			continue
		}

		// decode PTS
		if r.timedec == nil {
			r.timedec = mpegts.NewTimeDecoder(data.PES.Header.OptionalHeader.PTS.Base)
		}
		pts := r.timedec.Decode(data.PES.Header.OptionalHeader.PTS.Base)

		// decode H264 access unit
		au, err := h264.AnnexBUnmarshal(data.PES.Data)
		if err != nil {
			return nil, 0, err
		}

		return au, pts, nil
	}
}
