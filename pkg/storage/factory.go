// Package storage contains the storage mechanism of segments and parts.
package storage

// Factory allows to allocate the storage of segments and parts.
type Factory interface {
	NewSegment(fileName string) (Segment, error)
}
