// Package forward implements a forwarding proxy. It caches an upstream net.Conn for some time, so if the same
// client returns the upstream's Conn will be precached. Depending on how you benchmark this looks to be
// 50% faster than just openening a new connection for every client. It works with UDP and TCP and uses
// inband healthchecking.
package forward

import (
	"errors"
	"math/rand"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

// Forward represents a plugin instance that can proxy requests to another (DNS) server. It has a list
// of proxies each representing one upstream proxy.
type Forward struct {
	proxies []*proxy

	from       string
	ignored    []string
	forceTCP   bool          // here for testing
	hcInterval time.Duration // here for testing

	maxfails uint32

	Next plugin.Handler
}

// Name implements plugin.Handler.
func (f Forward) Name() string { return "forward" }

// ServeDNS implements plugin.Handler.
func (f Forward) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {

	state := request.Request{W: w, Req: r}
	if !f.match(state) {
		return plugin.NextOrFailure(f.Name(), f.Next, ctx, w, r)
	}

	for _, proxy := range f.selectp(w) {
		if !proxy.host.down(f.maxfails) {
			proxy.clientChan <- state

			RequestCount.WithLabelValues(state.Proto(), familyToString(state.Family()), proxy.host.String()).Add(1)
			SocketCount.WithLabelValues(proxy.host.String()).Set(float64(proxy.Len()))

			return 0, nil
		}
	}

	return dns.RcodeServerFailure, errNoHealthy
}

func (f Forward) match(state request.Request) bool {
	from := f.from

	if !plugin.Name(from).Matches(state.Name()) || !f.isAllowedDomain(state.Name()) {
		return false
	}

	return true
}

func (f Forward) isAllowedDomain(name string) bool {
	if dns.Name(name) == dns.Name(f.from) {
		return true
	}

	for _, ignore := range f.ignored {
		if plugin.Name(ignore).Matches(name) {
			return false
		}
	}
	return true
}

// selectp returns a randomized set of proxies to be used for this client. If the client was
// know to any of the proxies it will be put first.
func (f Forward) selectp(w dns.ResponseWriter) []*proxy {
	switch len(f.proxies) {
	case 1:
		return f.proxies
	case 2:
		id, _ := clientID(w)
		if f.proxies[0].knownClient(id) {
			return f.proxies // normal order
		}
		if f.proxies[1].knownClient(id) {
			return []*proxy{f.proxies[1], f.proxies[0]} // reverse
		}
		if rand.Int()%2 == 0 {
			return []*proxy{f.proxies[1], f.proxies[0]} // swap

		}
		return f.proxies // normal
	}

	id, _ := clientID(w)
	perms := rand.Perm(len(f.proxies))
	rnd := make([]*proxy, len(f.proxies))

	known := -1
	put := -1
	for i, p := range perms {
		if f.proxies[p].knownClient(id) {
			known = i
			put = p
		}

		println(i, p)
		rnd[i] = f.proxies[p]
	}
	// put known first, if we have one
	if known != -1 {
		x := rnd[1]
		rnd[1] = rnd[put]
		rnd[put] = x
	}

	return rnd
}

var (
	errInvalidDomain = errors.New("invalid domain for proxy")
	errNoHealthy     = errors.New("no healthy proxies")
)
