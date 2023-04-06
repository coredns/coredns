package atlas

import (
	"time"

	"github.com/coredns/coredns/request"
)

// Serial returns the serial number to use.
func (a *Atlas) Serial(state request.Request) uint32 {
	return uint32(time.Now().Unix())
}

// MinTTL returns the minimal TTL.
func (a *Atlas) MinTTL(state request.Request) uint32 {
	return 30
}
