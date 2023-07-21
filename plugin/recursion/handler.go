package recursion

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Recursion modifies flags of dns.MsgHdr in queries and / or responses
type Recursion struct {
	concurrent int64 // atomic counters need to be first in struct for proper alignment

	ignored       []string
	maxDepth      uint32
	maxTries      uint32
	timeout       time.Duration
	maxConcurrent int64

	Next plugin.Handler

	// ErrLimitExceeded indicates that a query was rejected because the number of concurrent queries has exceeded
	ErrLimitExceeded error
}

// New returns a new Recursion.
func New() *Recursion {
	f := &Recursion{maxDepth: 8, timeout: defaultTimeout, maxTries: 2}
	return f
}

// ServeDNS implements the plugin.Handler interface.
func (f Recursion) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	// Passthrough doing nothing if recursion is not desired in the request
	if !r.RecursionDesired || !f.match(state) || len(r.Question) != 1 {
		return plugin.NextOrFailure(f.Name(), f.Next, ctx, w, r)
	}

	// Limit the number of parallel recursion lookups
	if f.maxConcurrent > 0 {
		count := atomic.AddInt64(&(f.concurrent), 1)
		defer atomic.AddInt64(&(f.concurrent), -1)
		if count > f.maxConcurrent {
			maxConcurrentRejectCount.Add(1)
			return dns.RcodeRefused, f.ErrLimitExceeded
		}
	}

	// Set a timeout for recursion lookups, if specified, so that run away lookups don't consume resources.
	if f.timeout > 0 {
		ctx, cancel := context.WithTimeout(ctx, f.timeout)
	}

	// Create a responce writer, which is where the bulk of the handling is done.
	wr := ResponseRecursionWriter{
		ResponseWriter: w,
		maxDepth:       f.maxDepth,
		tries:          f.maxTries,
		qType:          r.Question[0].Qtype,
		qClass:         r.Question[0].Qclass,
		next:           f.Next,
		ctx:            ctx,
	}

	code, err := plugin.NextOrFailure(f.Name(), f.Next, ctx, &wr, r)

	cancel()

	return code, err
}

const name = "recursion"

// Name implements the plugin.Handler interface.
func (h Recursion) Name() string { return name }
