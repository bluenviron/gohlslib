package storage

import (
	"path/filepath"
)

type factoryDisk struct {
	dirPath string
}

// NewFactoryDisk allocates a disk-backed factory.
func NewFactoryDisk(dirPath string) Factory {
	return &factoryDisk{
		dirPath: dirPath,
	}
}

// NewSegment implements Factory.
func (s *factoryDisk) NewSegment(fileName string) (Segment, error) {
	return newSegmentDisk(filepath.Join(s.dirPath, fileName))
}
