package codecs

import (
	"sync"
)

// H264 is a H264 codec.
type H264 struct {
	SPS []byte
	PPS []byte

	mutex sync.RWMutex
}

func (*H264) isCodec() {}

// SafeSetParams sets the codec parameters.
func (c *H264) SafeSetParams(sps []byte, pps []byte) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.SPS = sps
	c.PPS = pps
}

// SafeParams returns the codec parameters.
func (c *H264) SafeParams() ([]byte, []byte) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.SPS, c.PPS
}
