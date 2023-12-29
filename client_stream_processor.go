package gohlslib

type clientStreamProcessor interface {
	getIsLeading() bool
	getTracks() []*Track
}
