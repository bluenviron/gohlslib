package storage

type factoryRAM struct{}

// NewFactoryRAM allocates a RAM-backed factory.
func NewFactoryRAM() Factory {
	return &factoryRAM{}
}

// NewSegment implements Factory.
func (s *factoryRAM) NewSegment(fileName string) (Segment, error) {
	return newSegmentRAM(), nil
}
