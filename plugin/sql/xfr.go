package sql

import (
	"time"

	"github.com/coredns/coredns/request"
)

// Serial returns the serial number to use.
func (s *SQL) Serial(state request.Request) uint32 {
	return uint32(time.Now().Unix())
}

// MinTTL returns the minimal TTL.
func (s *SQL) MinTTL(state request.Request) uint32 {
	return 30
}
