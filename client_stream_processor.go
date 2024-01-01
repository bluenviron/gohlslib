package gohlslib

import "time"

type clientStreamProcessor interface {
	getIsLeading() bool
	getTracks() []*Track
	ntp(dts time.Duration) (time.Time, bool)
}
