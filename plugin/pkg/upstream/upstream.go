// Package upstream abstracts a upstream lookups so that plugins can handle them in an unified way.
package upstream

import (
	"context"
	"errors"
	"fmt"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Upstream is used to resolve CNAME or other external targets via CoreDNS itself.
type Upstream struct{}

// New creates a new Upstream to resolve names using the coredns process.
func New() *Upstream { return &Upstream{} }

// maxDepth is the number of nested self-lookups one client query may cause.
// Each Lookup re-enters the server's plugin chain, and a plugin that answers
// with another CNAME re-enters it again. Backends serve data that other
// parties write (a Kubernetes tenant's Service and EndpointSlice, an etcd
// record), so a target that leads back to itself must stop here instead of
// growing the Go stack until the process dies. This matches the generic
// backend's CNAME-chain limit.
const maxDepth = 10

// errMaxDepth is returned when a lookup would nest deeper than maxDepth.
var errMaxDepth = errors.New("upstream lookup depth exceeded")

// depthKey is deliberately separate from dnsserver.LoopKey. The file and
// rewrite plugins already maintain LoopKey around their own CNAME processing;
// sharing it here would count each re-entry twice and shorten valid chains.
type depthKey struct{}

// Lookup routes lookups to our selves to make it follow the plugin chain *again*, but with a (possibly) new query. As
// we are doing the query against ourselves again, there is no actual new hop, as such RFC 6891 does not apply and we
// need the EDNS0 option present in the *original* query to be present here too.
//
// The nesting depth travels with the context so chains that alternate between
// plugins are bounded as one. Lookup returns errMaxDepth once maxDepth nested
// lookups are in progress for the client query.
func (u *Upstream) Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error) {
	server, ok := ctx.Value(dnsserver.Key{}).(*dnsserver.Server)
	if !ok {
		return nil, fmt.Errorf("no full server is running")
	}
	depth, _ := ctx.Value(depthKey{}).(int)
	if depth >= maxDepth {
		return nil, errMaxDepth
	}
	ctx = context.WithValue(ctx, depthKey{}, depth+1)

	req := state.NewWithQuestion(name, typ)

	nw := nonwriter.New(state.W)
	server.ServeDNS(ctx, nw, req.Req)

	return nw.Msg, nil
}
