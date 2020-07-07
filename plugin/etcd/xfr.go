package etcd

import (
	"time"

	"github.com/coredns/coredns/request"
)

// Serial returns the serial number to use.
func (e *Etcd) Serial(state request.Request) uint32 {
	return uint32(time.Now().Unix())
}

// MinTTL returns the minimal TTL.
func (e *Etcd) MinTTL(state request.Request) uint32 {
	return 30
}
