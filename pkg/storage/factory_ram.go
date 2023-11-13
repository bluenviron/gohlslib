package storage

type factoryRAM struct{}

// NewFactoryRAM allocates a RAM-backed factory.
func NewFactoryRAM() Factory {
	return &factoryRAM{}
}

// NewFile implements Factory.
func (s *factoryRAM) NewFile(_ string) (File, error) {
	return newFileRAM(), nil
}

func (s *factoryRAM) GetPath() (string) {
	return ""
}