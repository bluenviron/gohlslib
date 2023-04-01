package codecs

import (
	"sync"
)

// H265 is a H265 codec.
type H265 struct {
	VPS []byte
	SPS []byte
	PPS []byte

	mutex sync.RWMutex
}

func (*H265) isCodec() {}

// SafeSetParams sets the codec parameters.
func (c *H265) SafeSetParams(vps []byte, sps []byte, pps []byte) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.VPS = vps
	c.SPS = sps
	c.PPS = pps
}

// SafeParams returns the codec parameters.
func (c *H265) SafeParams() ([]byte, []byte, []byte) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.VPS, c.SPS, c.PPS
}
