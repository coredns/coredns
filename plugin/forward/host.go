package forward

import "sync"

type host struct {
	addr string

	fails uint32
	sync.RWMutex
	checking bool
}

func newHost(addr string) *host { return &host{addr: addr} }

func (h *host) String() string { return h.addr }
