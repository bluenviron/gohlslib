package codecparams

import (
	"strings"
)

// CheckSupport checks whether codec parameters are supported by this library.
func CheckSupport(codecParams string) bool {
	for _, codec := range strings.Split(codecParams, ",") {
		if !strings.HasPrefix(codec, "avc1.") &&
			!strings.HasPrefix(codec, "hvc1.") &&
			!strings.HasPrefix(codec, "hev1.") &&
			!strings.HasPrefix(codec, "mp4a.") &&
			codec != "opus" {
			return false
		}
	}
	return true
}
