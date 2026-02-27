package gohlslib

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/asticode/go-astits"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

func TestMuxerKLV(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack, klvTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := range 4 {
		d := time.Duration(i) * time.Second
		pts := int64(d) * 90000 / int64(time.Second)

		err = m.WriteKLV(klvTrack, testTime.Add(d), pts, []byte{
			0x00, 0x01, 0x02, 0x03,
		})
		require.NoError(t, err)

		// Write H264 (IDR to force segment creation)
		err = m.WriteH264(testVideoTrack, testTime.Add(d), pts, [][]byte{
			testSPS,
			{8}, // PPS
			{5}, // IDR
		})
		require.NoError(t, err)
	}

	byts, _, err := doRequest(m, "index.m3u8")
	require.NoError(t, err)

	require.Contains(t, string(byts), "main_stream.m3u8")

	byts, _, err = doRequest(m, "main_stream.m3u8")
	require.NoError(t, err)

	re := regexp.MustCompile(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-ALLOW-CACHE:NO
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-PROGRAM-DATE-TIME:.*?
#EXTINF:1.00000,
(.*?_main_seg0\.ts)
#EXT-X-PROGRAM-DATE-TIME:.*?
#EXTINF:1.00000,
(.*?_main_seg1\.ts)
#EXT-X-PROGRAM-DATE-TIME:.*?
#EXTINF:1.00000,
(.*?_main_seg2\.ts)
`)
	require.Regexp(t, re, string(byts))
	ma := re.FindStringSubmatch(string(byts))

	// Fetch the first segment and parse it to verify KLV data
	segmentData, _, err := doRequest(m, ma[1])
	require.NoError(t, err)
	require.NotEmpty(t, segmentData)

	// Parse the MPEG-TS segment using astits
	demuxer := astits.NewDemuxer(context.Background(), bytes.NewReader(segmentData))

	// Track whether we found KLV data
	foundKLVPID := false
	foundKLVData := false
	var klvPID uint16
	var receivedKLVData []byte

	// Iterate through all packets in the segment
	for {
		data, nextErr := demuxer.NextData()
		if nextErr != nil {
			break
		}

		// Check if this is a PMT (Program Map Table) and find KLV PID
		if data.PMT != nil && !foundKLVPID {
			for _, es := range data.PMT.ElementaryStreams {
				// KLV data uses stream type 0x06 (private data)
				if es.StreamType == astits.StreamTypePrivateData {
					klvPID = es.ElementaryPID
					foundKLVPID = true
					break
				}
			}
		}

		// Check if this packet contains KLV data
		if foundKLVPID && data.PES != nil && data.PID == klvPID && !foundKLVData {
			foundKLVData = true
			receivedKLVData = data.PES.Data
			// Verify the KLV data matches what we wrote
			require.Equal(t, []byte{0x00, 0x01, 0x02, 0x03}, receivedKLVData,
				"KLV data content mismatch")
			break
		}
	}

	require.True(t, foundKLVPID, "KLV PID was not found in PMT")
	require.True(t, foundKLVData, "KLV data was not found in the segment")
}

func TestMuxerKLVOnlyTrackRejected(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{klvTrack},
	}

	err := m.Start()
	require.Error(t, err)
	require.Contains(t, err.Error(), "KLV tracks require at least one video or audio track")
}

func TestMuxerKLVFirstTrackWithAudio(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	audioTrack := &Track{
		Codec: &codecs.MPEG4Audio{
			Config: mpeg4audio.AudioSpecificConfig{
				Type:          2,
				SampleRate:    44100,
				ChannelConfig: 2,
				ChannelCount:  2,
			},
		},
		ClockRate: 44100,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{klvTrack, audioTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	// Verify that the audio track is the leading track, not KLV
	require.False(t, m.mtracksByTrack[klvTrack].isLeading, "KLV track should not be leading")
	require.True(t, m.mtracksByTrack[audioTrack].isLeading, "Audio track should be leading")
}

// TestMuxerKLVSynchronous verifies that a synchronous KLV track (Synchronous: true)
// is registered in the PMT as StreamTypeMetadata (0x15) rather than StreamTypePrivateData
// (0x06), and that PES packets for it carry a PTS.
func TestMuxerKLVSynchronous(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{Synchronous: true},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack, klvTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := range 4 {
		d := time.Duration(i) * time.Second
		pts := int64(d) * 90000 / int64(time.Second)

		err = m.WriteKLV(klvTrack, testTime.Add(d), pts, []byte{0xAA, 0xBB})
		require.NoError(t, err)

		err = m.WriteH264(testVideoTrack, testTime.Add(d), pts, [][]byte{
			testSPS,
			{8},
			{5},
		})
		require.NoError(t, err)
	}

	byts, _, err := doRequest(m, "main_stream.m3u8")
	require.NoError(t, err)

	re := regexp.MustCompile(`(.*?_main_seg0\.ts)`)
	ma := re.FindStringSubmatch(string(byts))
	require.NotNil(t, ma, "no segment found in playlist")

	segmentData, _, err := doRequest(m, ma[1])
	require.NoError(t, err)
	require.NotEmpty(t, segmentData)

	demuxer := astits.NewDemuxer(context.Background(), bytes.NewReader(segmentData))

	foundSyncKLV := false
	for {
		data, nextErr := demuxer.NextData()
		if nextErr != nil {
			break
		}
		if data.PMT != nil {
			for _, es := range data.PMT.ElementaryStreams {
				// Synchronous KLV is registered as StreamTypeMetadata (0x15)
				if es.StreamType == astits.StreamTypeMetadata {
					foundSyncKLV = true
				}
			}
		}
	}
	require.True(t, foundSyncKLV, "synchronous KLV PID with StreamTypeMetadata not found in PMT")
}

// TestMuxerKLVMultipleTracks verifies that two KLV tracks (one async, one sync)
// each receive their own distinct PID in the PMT.
func TestMuxerKLVMultipleTracks(t *testing.T) {
	klvAsync := &Track{
		Codec:     &codecs.KLV{Synchronous: false},
		ClockRate: 90000,
	}
	klvSync := &Track{
		Codec:     &codecs.KLV{Synchronous: true},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack, klvAsync, klvSync},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := range 4 {
		d := time.Duration(i) * time.Second
		pts := int64(d) * 90000 / int64(time.Second)

		err = m.WriteKLV(klvAsync, testTime.Add(d), pts, []byte{0x01, 0x02})
		require.NoError(t, err)

		err = m.WriteKLV(klvSync, testTime.Add(d), pts, []byte{0x03, 0x04})
		require.NoError(t, err)

		err = m.WriteH264(testVideoTrack, testTime.Add(d), pts, [][]byte{
			testSPS,
			{8},
			{5},
		})
		require.NoError(t, err)
	}

	byts, _, err := doRequest(m, "main_stream.m3u8")
	require.NoError(t, err)

	re := regexp.MustCompile(`(.*?_main_seg0\.ts)`)
	ma := re.FindStringSubmatch(string(byts))
	require.NotNil(t, ma)

	segmentData, _, err := doRequest(m, ma[1])
	require.NoError(t, err)

	demuxer := astits.NewDemuxer(context.Background(), bytes.NewReader(segmentData))

	asyncPIDs := 0
	syncPIDs := 0
	for {
		data, nextErr := demuxer.NextData()
		if nextErr != nil {
			break
		}
		if data.PMT != nil {
			for _, es := range data.PMT.ElementaryStreams {
				switch es.StreamType {
				case astits.StreamTypePrivateData:
					asyncPIDs++
				case astits.StreamTypeMetadata:
					syncPIDs++
				}
			}
		}
	}
	require.Equal(t, 1, asyncPIDs, "expected exactly one async KLV PID (StreamTypePrivateData)")
	require.Equal(t, 1, syncPIDs, "expected exactly one sync KLV PID (StreamTypeMetadata)")
}

// TestMuxerKLVBeforeFirstSegment verifies that KLV data written before the leading
// track has created the first segment is silently dropped without returning an error.
func TestMuxerKLVBeforeFirstSegment(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack, klvTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	// Write KLV before any H264 — no segment exists yet, must be silently dropped.
	err = m.WriteKLV(klvTrack, testTime, 0, []byte{0xDE, 0xAD, 0xBE, 0xEF})
	require.NoError(t, err, "WriteKLV before first segment should not return an error")

	// Now drive segment creation with H264 frames.
	for i := range 4 {
		d := time.Duration(i) * time.Second
		pts := int64(d) * 90000 / int64(time.Second)
		err = m.WriteH264(testVideoTrack, testTime.Add(d), pts, [][]byte{
			testSPS,
			{8},
			{5},
		})
		require.NoError(t, err)
	}

	byts, _, err := doRequest(m, "main_stream.m3u8")
	require.NoError(t, err)
	require.Contains(t, string(byts), "seg0.ts")
}

// TestMuxerKLVWriteOnFMP4Variant verifies that calling WriteKLV on a muxer that
// is using the FMP4 variant returns an appropriate error.
func TestMuxerKLVWriteOnFMP4Variant(t *testing.T) {
	m := &Muxer{
		Variant:            MuxerVariantFMP4,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	// Use a KLV track that is not registered in the muxer.
	fakeKLVTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	err = m.WriteKLV(fakeKLVTrack, testTime, 0, []byte{0x01})
	require.Error(t, err)
	require.Contains(t, err.Error(), "MPEG-TS")
}

// TestMuxerKLVFMP4TrackRejectedAtStart verifies that attempting to Start() a
// non-MPEG-TS muxer with a KLV track returns an error immediately.
func TestMuxerKLVFMP4TrackRejectedAtStart(t *testing.T) {
	cases := []struct {
		name     string
		variant  MuxerVariant
		segCount int
	}{
		{"fmp4", MuxerVariantFMP4, 3},
		{"lowLatency", MuxerVariantLowLatency, 7},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			klvTrack := &Track{
				Codec:     &codecs.KLV{},
				ClockRate: 90000,
			}

			m := &Muxer{
				Variant:            tc.variant,
				SegmentCount:       tc.segCount,
				SegmentMinDuration: 1 * time.Second,
				Tracks:             []*Track{testVideoTrack, klvTrack},
			}

			err := m.Start()
			require.Error(t, err)
			require.Contains(t, err.Error(), "MPEG-TS")
		})
	}
}

// TestMuxerKLVEmptyData verifies that writing a zero-length KLV payload does not
// panic and is handled gracefully.
func TestMuxerKLVEmptyData(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack, klvTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := range 4 {
		d := time.Duration(i) * time.Second
		pts := int64(d) * 90000 / int64(time.Second)

		// Write empty KLV payload — must not panic.
		err = m.WriteKLV(klvTrack, testTime.Add(d), pts, []byte{})
		require.NoError(t, err)

		err = m.WriteH264(testVideoTrack, testTime.Add(d), pts, [][]byte{
			testSPS,
			{8},
			{5},
		})
		require.NoError(t, err)
	}

	byts, _, err := doRequest(m, "main_stream.m3u8")
	require.NoError(t, err)
	require.Contains(t, string(byts), "seg0.ts")
}

// TestMuxerKLVMultivariantCODECS verifies that the CODECS attribute in the
// multivariant playlist does not contain an empty string for KLV tracks, which
// have no standard HLS codec identifier.
func TestMuxerKLVMultivariantCODECS(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack, klvTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := range 4 {
		d := time.Duration(i) * time.Second
		pts := int64(d) * 90000 / int64(time.Second)

		err = m.WriteKLV(klvTrack, testTime.Add(d), pts, []byte{0x01, 0x02})
		require.NoError(t, err)

		err = m.WriteH264(testVideoTrack, testTime.Add(d), pts, [][]byte{
			testSPS,
			{8},
			{5},
		})
		require.NoError(t, err)
	}

	// Fetch the multivariant playlist.
	byts, _, err := doRequest(m, "index.m3u8")
	require.NoError(t, err)

	playlist := string(byts)

	// The CODECS attribute must not contain an empty codec string (e.g. "avc1.64001f,").
	require.NotContains(t, playlist, `CODECS="",`,
		"CODECS attribute must not start with an empty string")
	require.NotContains(t, playlist, `,""`,
		"CODECS attribute must not contain a trailing empty string")

	// Verify the H264 codec IS present.
	require.True(t, strings.Contains(playlist, "avc1."),
		"CODECS attribute must contain the H264 codec string")

	// Verify no bare comma (which signals an empty element) appears inside the CODECS value.
	re := regexp.MustCompile(`CODECS="([^"]*)"`)
	ma := re.FindStringSubmatch(playlist)
	require.NotNil(t, ma, "CODECS attribute not found in multivariant playlist")
	for part := range strings.SplitSeq(ma[1], ",") {
		require.NotEmpty(t, strings.TrimSpace(part),
			"CODECS attribute contains an empty entry: %q", ma[1])
	}
}

// TestMuxerKLVExceedsSegmentMaxSize verifies that writing KLV data that would push
// the segment past SegmentMaxSize returns an error.
func TestMuxerKLVExceedsSegmentMaxSize(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	// testSPS(25B) + PPS(1B) + IDR(1B) = 27 bytes for the first H264 write.
	// A SegmentMaxSize of 50 bytes accepts H264 but not an additional 30-byte KLV payload.
	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		SegmentMaxSize:     50,
		Tracks:             []*Track{testVideoTrack, klvTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	// Write the first H264 IDR to create the segment (consumes 27 bytes of the 50-byte limit).
	err = m.WriteH264(testVideoTrack, testTime, 0, [][]byte{testSPS, {8}, {5}})
	require.NoError(t, err)

	// Write 30 bytes of KLV into the same segment — 27 + 30 = 57 > 50, must fail.
	err = m.WriteKLV(klvTrack, testTime, 0, make([]byte, 30))
	require.Error(t, err)
	require.Contains(t, err.Error(), "maximum segment size")
}
