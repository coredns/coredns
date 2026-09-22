// Package shed bounds concurrent UDP response writes per listener socket so
// that an overload storm degrades into counted drops instead of a goroutine
// pile-up and the Go runtime's fdMutex overflow panic. See README.md for the
// failure mode and fdmutex_test.go for its reproduction.
//
// Setup installs dnsserver's Config.UDPDecorateWriterFunc: the decorated
// Write pushes the packed response onto a bounded evict-oldest stack and one
// writer goroutine per socket pops newest-first onto the wire, so the fd
// never sees more than one writer. While a socket's stack is full, ServeDNS
// drops arrivals before any chain work. Every drop is counted.
package shed

import (
	"context"
	"sync"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

// Shed implements the plugin.Handler interface.
type Shed struct {
	Next plugin.Handler
}

// registry maps a listener socket's *dnsserver.Server to its per-socket
// state. It is package level: several server blocks can share a listener,
// and every block's ServeDNS must see that socket's one stack.
var registry sync.Map // *dnsserver.Server -> *socketState

// socketState is one listener socket's registry record. owner scopes
// shutdown to this instance's entries on reload.
type socketState struct {
	owner        *Shed
	stack        *respStack
	droppedQuery prometheus.Counter
}

// lookupState returns the per-socket state for this request's socket, or nil
// — and on nil ServeDNS fails open. A miss happens when the request carries
// no *dnsserver.Server in its context (tests, non-dnsserver entry points),
// or for a handler still finishing on a server a reload already removed;
// neither admits unbounded new work.
func (s *Shed) lookupState(ctx context.Context) *socketState {
	srv := ctx.Value(dnsserver.Key{})
	if srv == nil {
		return nil
	}
	if v, ok := registry.Load(srv); ok {
		return v.(*socketState)
	}
	return nil
}

// mintState creates one listener socket's state and starts its writer
// goroutine. Idempotent; the loaded path only happens in tests.
func (s *Shed) mintState(srv *dnsserver.Server) *socketState {
	st := &socketState{
		owner:        s,
		stack:        newRespStack(stackDepth, droppedTotal.WithLabelValues(srv.Address(), "response")),
		droppedQuery: droppedTotal.WithLabelValues(srv.Address(), "query"),
	}
	if v, loaded := registry.LoadOrStore(srv, st); loaded {
		return v.(*socketState)
	}
	go st.stack.writerLoop()
	return st
}

// shutdown removes this instance's registry entries and stops their writer
// goroutines. A push after removal is rejected by the closed stack and
// counted as a dropped response.
func (s *Shed) shutdown() error {
	registry.Range(func(k, v any) bool {
		if st := v.(*socketState); st.owner == s {
			// LoadAndDelete makes close-once structural even if sweeps race.
			if _, loaded := registry.LoadAndDelete(k); loaded {
				st.stack.close()
			}
		}
		return true
	})
	return nil
}

// ServeDNS implements the plugin.Handler interface.
func (s *Shed) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	// The failure mechanism is UDP-specific: connectionless writes racing
	// one fdMutex. TCP must never be starved by UDP-storm shedding.
	state := request.Request{W: w, Req: r}
	if state.Proto() != "udp" {
		return plugin.NextOrFailure(s.Name(), s.Next, ctx, w, r)
	}
	st := s.lookupState(ctx)
	if st == nil {
		return plugin.NextOrFailure(s.Name(), s.Next, ctx, w, r) // fail open — see lookupState
	}

	// Coupled shed. full() is a racy read by design: a load-shedding
	// heuristic, not an invariant. Silent drop: nothing is written, and
	// RcodeSuccess satisfies plugin.ClientWrite so dnsserver writes nothing
	// either.
	if st.stack.full() {
		st.droppedQuery.Inc()
		return dns.RcodeSuccess, nil
	}

	return plugin.NextOrFailure(s.Name(), s.Next, ctx, w, r)
}

// Name implements the plugin.Handler interface.
func (s *Shed) Name() string { return pluginName }
