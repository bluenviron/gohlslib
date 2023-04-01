// Package codecparams contains utilities to deal with codec parameters.
package codecparams

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
)

func encodeProfileSpace(v uint8) string {
	switch v {
	case 1:
		return "A"
	case 2:
		return "B"
	case 3:
		return "C"
	}
	return ""
}

func encodeCompatibilityFlag(v [32]bool) string {
	var o uint32
	for i, b := range v {
		if b {
			o |= 1 << i
		}
	}
	return fmt.Sprintf("%x", o)
}

func encodeGeneralTierFlag(v uint8) string {
	if v > 0 {
		return "H"
	}
	return "L"
}

func encodeGeneralConstraintIndicatorFlags(v *h265.SPS_ProfileTierLevel) string {
	var ret []string

	var o1 uint8
	if v.GeneralProgressiveSourceFlag {
		o1 |= 1 << 7
	}
	if v.GeneralInterlacedSourceFlag {
		o1 |= 1 << 6
	}
	if v.GeneralNonPackedConstraintFlag {
		o1 |= 1 << 5
	}
	if v.GeneralFrameOnlyConstraintFlag {
		o1 |= 1 << 4
	}
	if v.GeneralMax12bitConstraintFlag {
		o1 |= 1 << 3
	}
	if v.GeneralMax10bitConstraintFlag {
		o1 |= 1 << 2
	}
	if v.GeneralMax8bitConstraintFlag {
		o1 |= 1 << 1
	}
	if v.GeneralMax422ChromeConstraintFlag {
		o1 |= 1 << 0
	}

	ret = append(ret, fmt.Sprintf("%x", o1))

	var o2 uint8
	if v.GeneralMax420ChromaConstraintFlag {
		o2 |= 1 << 7
	}
	if v.GeneralMaxMonochromeConstraintFlag {
		o2 |= 1 << 6
	}
	if v.GeneralIntraConstraintFlag {
		o2 |= 1 << 5
	}
	if v.GeneralOnePictureOnlyConstraintFlag {
		o2 |= 1 << 4
	}
	if v.GeneralLowerBitRateConstraintFlag {
		o2 |= 1 << 3
	}
	if v.GeneralMax14BitConstraintFlag {
		o2 |= 1 << 2
	}

	if o2 != 0 {
		ret = append(ret, fmt.Sprintf("%x", o2))
	}

	return strings.Join(ret, ".")
}

// Marshal generates codec parameters of given tracks.
func Marshal(codec codecs.Codec) string {
	switch tcodec := codec.(type) {
	case *codecs.H264:
		if len(tcodec.SPS) >= 4 {
			return "avc1." + hex.EncodeToString(tcodec.SPS[1:4])
		}

	case *codecs.H265:
		var sps h265.SPS
		err := sps.Unmarshal(tcodec.SPS)
		if err == nil {
			return "hvc1." +
				encodeProfileSpace(sps.ProfileTierLevel.GeneralProfileSpace) +
				strconv.FormatInt(int64(sps.ProfileTierLevel.GeneralProfileIdc), 10) + "." +
				encodeCompatibilityFlag(sps.ProfileTierLevel.GeneralProfileCompatibilityFlag) + "." +
				encodeGeneralTierFlag(sps.ProfileTierLevel.GeneralTierFlag) +
				strconv.FormatInt(int64(sps.ProfileTierLevel.GeneralLevelIdc), 10) + "." +
				encodeGeneralConstraintIndicatorFlags(&sps.ProfileTierLevel)
		}

	case *codecs.MPEG4Audio:
		// https://developer.mozilla.org/en-US/docs/Web/Media/Formats/codecs_parameter
		return "mp4a.40." + strconv.FormatInt(int64(tcodec.Config.Type), 10)

	case *codecs.Opus:
		return "opus"
	}

	return ""
}
