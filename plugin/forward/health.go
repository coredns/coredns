package forward

import (
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// For HC we send to . IN NS +norec message to the upstream. Dial timeouts and empty
// replies are considered fails, basically anything else constitutes a healthy upstream.

func (h *host) Check() {
	h.Lock()

	if h.checking {
		h.Unlock()
		return
	}

	h.checking = true
	h.Unlock()

	err := h.send()
	if err != nil {
		atomic.AddUint32(&h.fails, 1)
	} else {
		atomic.StoreUint32(&h.fails, 0)
	}

	h.Lock()
	h.checking = false
	h.Unlock()

	return
}

func (h *host) send() error {
	hcping := new(dns.Msg)
	hcping.SetQuestion(".", dns.TypeNS)
	hcping.RecursionDesired = false

	_, _, err := hcclient.Exchange(hcping, h.addr)
	return err
}

func (h *host) down(maxfails uint32) bool {
	if maxfails == 0 {
		return false
	}

	fails := atomic.LoadUint32(&h.fails)
	return fails > maxfails
}

var hcclient = func() *dns.Client {
	c := new(dns.Client)
	c.Net = "tcp"
	c.ReadTimeout = 2 * time.Second
	c.WriteTimeout = 2 * time.Second
	return c
}()
