package cache

import (
	"context"
	"math"
	"net"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/cache"
	"github.com/coredns/coredns/request"

	"github.com/infobloxopen/go-trees/iptree"
	"github.com/miekg/dns"
)

// Use ::/0 as wildcard key for queries without ECS (both v4 and v6)
var zeroSubnet = net.IPNet{IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}

// Use 0.0.0.0/0 as wildcard key for private ECS queries (both v4 and v6)
// (i.e. those were the client explicitly asked via ECS to not pass our IP to upstreams)
var privateZeroSubnet = net.IPNet{IP: net.IPv4zero.To4(), Mask: net.CIDRMask(0, 32)}

// ServeDNS implements the plugin.Handler interface.
func (c *Cache) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	// The cache only handles QUERY messages. In particular, an UPDATE can
	// have the same zone name and QTYPE as a cached query, but must always
	// reach the authoritative handler instead of being answered from cache.
	if r.Opcode != dns.OpcodeQuery {
		return plugin.NextOrFailure(c.Name(), c.Next, ctx, w, r)
	}

	rc := r.Copy() // We potentially modify r, to prevent other plugins from seeing this (r is a pointer), copy r into rc.
	state := request.Request{W: w, Req: rc}
	do := state.Do()
	cd := r.CheckingDisabled
	ad := r.AuthenticatedData

	if !plugin.Zones(c.Zones).Contains(state.Name()) || plugin.Zones(c.bypass).Contains(state.Name()) {
		return plugin.NextOrFailure(c.Name(), c.Next, ctx, w, rc)
	}

	now := c.now()
	server := metrics.WithServer(ctx)

	// On cache refresh, we will just use the DO bit from the incoming query for the refresh since we key our cache
	// with the query DO bit. That means two separate cache items for the query DO bit true or false. In the situation
	// in which upstream doesn't support DNSSEC, the two cache items will effectively be the same. Regardless, any
	// DNSSEC RRs in the response are written to cache with the response.

	subnet := &zeroSubnet
	o := r.IsEdns0()
	var ecs *dns.EDNS0_SUBNET
	var ok bool
	exactMatch := &zeroSubnet
	if o != nil {
		for _, s := range o.Option {
			if ecs, ok = s.(*dns.EDNS0_SUBNET); ok {
				// Section 7.1.1, "Recursive Resolvers".
				// If the triggering query included an ECS option itself, it MUST be
				// examined for its SOURCE PREFIX-LENGTH.  The Recursive Resolver's
				// outgoing query MUST then set SOURCE PREFIX-LENGTH to the shorter of
				// the incoming query's SOURCE PREFIX-LENGTH or the server's maximum
				// cacheable prefix length.

				// Section 7.1.3, "Forwarding Resolvers".
				// Forwarding Resolvers essentially appear to be Stub Resolvers to
				// whatever Recursive Resolver is ultimately handling the query, but
				// they look like a Recursive Resolver to their client.  A Forwarding
				// Resolver using this option MUST prepare it as described in
				// Section 7.1.1, "Recursive Resolvers".

				// Normalize
				if temp := ecs.Address.To4(); temp != nil {
					ecs.Address = temp
				}

				var mask net.IPMask
				if ecs.Family == 1 {
					// If SOURCE PREFIX-LENGTH is shorter than the configured maximum and
					// SCOPE PREFIX-LENGTH is longer than SOURCE PREFIX-LENGTH, store SOURCE
					// PREFIX-LENGTH bits of ADDRESS, and then mark the response as valid
					// only to answer client queries that specify exactly the same SOURCE
					// PREFIX-LENGTH in their own ECS option.
					//
					// Weirdly, this means to cache by the requested prefix, instead of the returned one
					// i.e. req: 10.0.0.0/8 (max is 16), response covers only 10.0.0.0/24, cache for 10.0.0.0/8
					// and only for queries that have an ECS option with subnet /8 and address 10.0.0.0, i.e.
					// **exact matches only**, do NOT cache for 10.0.0.0/24 or 10.0.1.0/24 even if it falls inside of 10.0.0.0/8
					//
					if ecs.SourceNetmask < c.mask_v4_size {
						exactMatch = &net.IPNet{IP: ecs.Address, Mask: net.CIDRMask(int(ecs.SourceNetmask), 32)}
					}
					ecs.SourceNetmask = min(ecs.SourceNetmask, c.mask_v4_size)
					mask = net.CIDRMask(int(ecs.SourceNetmask), 32)
				} else {
					if ecs.SourceNetmask < c.mask_v6_size {
						exactMatch = &net.IPNet{IP: ecs.Address, Mask: net.CIDRMask(int(ecs.SourceNetmask), 128)}
					}
					ecs.SourceNetmask = min(ecs.SourceNetmask, c.mask_v6_size)
					mask = net.CIDRMask(int(ecs.SourceNetmask), 128)
				}
				ecs.Address = ecs.Address.Mask(mask)
				if ecs.SourceNetmask == 0 {
					subnet = &privateZeroSubnet
				} else {
					subnet = &net.IPNet{IP: ecs.Address, Mask: mask}
				}
				break
			}
		}
	}

	var srcOrig net.IP
	ip := w.RemoteAddr()
	if i, ok := ip.(*net.UDPAddr); ok {
		srcOrig = i.IP
	}
	if i, ok := ip.(*net.TCPAddr); ok {
		srcOrig = i.IP
	}

	// Normalize
	if temp := srcOrig.To4(); temp != nil {
		srcOrig = temp
	}

	i := c.getIfNotStale(now, state, subnet, exactMatch, srcOrig, server)
	if i == nil {
		refreshState := authenticatedRefreshState(state)
		crr := &ResponseWriter{ResponseWriter: w, Cache: c, state: refreshState, server: server, do: do, ad: ad, cd: cd,
			subnet:     subnet,
			exactMatch: exactMatch,
			ecs:        ecs,
			nexcept:    c.nexcept, pexcept: c.pexcept, wildcardFunc: wildcardFunc(ctx)}
		ret, err := c.doRefresh(ctx, refreshState, crr)
		// The next plugin can fail without writing a response, e.g. forward when
		// the upstream times out. In that case the failure never reaches
		// ResponseWriter.WriteMsg, so it is not cached and every client retry
		// hits the upstream again. Store a SERVFAIL bounded by the servfail TTL
		// so identical queries are answered from the cache while it is valid.
		// A canceled request context (e.g. a canceled DoH request) is not an
		// upstream failure, so it must not poison the shared failure cache.
		if err != nil && ret == dns.RcodeServerFailure && ctx.Err() == nil && crr.lastResponse == nil && c.failttl > 0 {
			crr.cacheServerFailure(refreshState.Req)
		}
		return ret, err
	}
	stale := !i.withinLifetime(now, 0)
	if stale {
		// serve stale behavior
		if c.verifyStale {
			failureRecheck := c.staleRecheck
			nowFunc := c.now
			trackRefresh := failureRecheck > 0 || c.verifyStaleTimeout > 0
			if !trackRefresh || i.beginRefresh(now, failureRecheck) {
				refreshState := authenticatedRefreshState(state)
				crr := &ResponseWriter{ResponseWriter: w, Cache: c, state: refreshState, server: server, do: do, ad: ad, cd: cd,
					subnet:     subnet,
					exactMatch: exactMatch,
					ecs:        ecs}
				if c.verifyStaleTimeout > 0 {
					// Background verify: cache the response but do not write to the wire.
					// On timeout, we serve the stale entry below and let the goroutine continue.
					crr.prefetch = true
				}
				cw := newVerifyStaleResponseWriter(crr)
				if c.verifyStaleTimeout == 0 {
					ret, err := c.doRefresh(ctx, refreshState, cw)
					if trackRefresh {
						i.endRefresh(nowFunc(), failureRecheck, cw.refreshed)
					}
					if cw.refreshed {
						return ret, err
					}
				} else if served, ret, err := c.verifyWithTimeout(ctx, refreshState, w, cw, r, do, ad, i, failureRecheck, nowFunc); served {
					return ret, err
				}
			}
		}

		// Adjust the time to get a 0 TTL in the reply built from a stale item.
		now = now.Add(time.Duration(i.ttl(now)) * time.Second)
		if !c.verifyStale {
			c.tryPrefetch(ctx, i, server, rc, do, cd, subnet, exactMatch, ecs, srcOrig, now, true)
		}
		servedStale.WithLabelValues(server, c.zonesMetricLabel, c.viewMetricLabel).Inc()
	} else if c.shouldPrefetch(i, now) {
		c.tryPrefetch(ctx, i, server, rc, do, cd, subnet, exactMatch, ecs, srcOrig, now, false)
	}

	if i.wildcard != "" {
		// Set wildcard source record name to metadata
		metadata.SetValueFunc(ctx, "zone/wildcard", func() string {
			return i.wildcard
		})
	}

	if c.keepttl {
		// If keepttl is enabled we fake the current time to the stored
		// one so that we always get the original TTL
		now = i.stored
	}
	var resp *dns.Msg
	if stale && c.staleTTL > 0 {
		staleTTL := uint32(c.staleTTL / time.Second) // #nosec G115 -- configuration parsing bounds this to the DNS TTL range.
		resp = i.toMsgWithTTL(r, staleTTL, do, ad)
	} else {
		resp = i.toMsg(r, now, do, ad)
	}
	w.WriteMsg(resp)
	return dns.RcodeSuccess, nil
}

func wildcardFunc(ctx context.Context) func() string {
	return func() string {
		// Get wildcard source record name from metadata
		if f := metadata.ValueFunc(ctx, "zone/wildcard"); f != nil {
			return f()
		}
		return ""
	}
}

// tryPrefetch dispatches a background prefetch for i if one is not already in
// flight. The CAS on i.refreshing ensures at most one prefetch goroutine per
// item, so prefetch load scales with distinct stale keys rather than QPS.
func (c *Cache) tryPrefetch(ctx context.Context, i *item, server string, req *dns.Msg, do, cd bool, subnet, exactMatch *net.IPNet, ecs *dns.EDNS0_SUBNET, srcOrig net.IP, now time.Time, stale bool) {
	failureRecheck := time.Duration(0)
	if stale {
		failureRecheck = c.staleRecheck
	}
	nowFunc := c.now
	if !i.beginRefresh(nowFunc(), failureRecheck) {
		return
	}
	cw := newPrefetchResponseWriter(server, req, do, cd, subnet, exactMatch, ecs, c)
	go func() {
		refreshed := c.doPrefetch(ctx, cw, i, srcOrig, now, stale)
		i.endRefresh(nowFunc(), failureRecheck, refreshed)
	}()
}

func (c *Cache) doPrefetch(ctx context.Context, cw *ResponseWriter, i *item, srcOrig net.IP, now time.Time, stale bool) bool {
	// Use a fresh metadata map to avoid concurrent writes to the original request's metadata.
	ctx = metadata.ContextWithMetadata(ctx)
	cachePrefetches.WithLabelValues(cw.server, c.zonesMetricLabel, c.viewMetricLabel).Inc()
	refreshed := true
	if stale {
		refreshWriter := newVerifyStaleResponseWriter(cw)
		c.doRefresh(ctx, cw.state, refreshWriter)
		refreshed = refreshWriter.refreshed
	} else {
		c.doRefresh(ctx, cw.state, cw)
	}
	if !refreshed {
		return false
	}

	// When prefetching we loose the item i, and with it the frequency
	// that we've gathered sofar. See we copy the frequencies info back
	// into the new item that was stored in the cache.
	if i1 := c.exists(cw.state.Name(), cw.state.QType(), cw.state.QClass(), cw.do, cw.cd, cw.subnet, cw.exactMatch, srcOrig); i1 != nil {
		i1.Reset(now, i.Hits())
	}
	return true
}

func (c *Cache) doRefresh(ctx context.Context, state request.Request, cw dns.ResponseWriter) (int, error) {
	return plugin.NextOrFailure(c.Name(), c.Next, ctx, cw, state.Req)
}

// verifyWithTimeout runs the upstream verify in a background goroutine and races it
// against verifyStaleTimeout. If the verify completes within the timeout and the
// response is accepted by verifyStaleResponseWriter, the freshly cached entry is
// served to the client and served is true. Otherwise served is false and the caller
// falls through to serve stale; the goroutine continues to run and any cacheable
// response updates the cache without writing to the detached client connection.
func (c *Cache) verifyWithTimeout(ctx context.Context, state request.Request, w dns.ResponseWriter, cw *verifyStaleResponseWriter, r *dns.Msg, do, ad bool, i *item, failureRecheck time.Duration, now func() time.Time) (served bool, code int, err error) {
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	refreshCtx := metadata.ContextWithMetadata(ctx)
	for label, valueFunc := range metadata.ValueFuncs(ctx) {
		metadata.SetValueFunc(refreshCtx, label, valueFunc)
	}
	go func() {
		rc, re := c.doRefresh(refreshCtx, state, cw)
		i.endRefresh(now(), failureRecheck, cw.refreshed)
		done <- result{rc, re}
	}()
	timer := time.NewTimer(c.verifyStaleTimeout)
	defer timer.Stop()
	select {
	case res := <-done:
		if !cw.refreshed {
			return false, 0, nil
		}
		if cw.response == nil {
			return true, res.code, res.err
		}
		response := cw.response
		if cw.item != nil {
			now := c.now()
			if c.keepttl {
				now = cw.item.stored
			}
			response = cw.item.toMsg(r, now, do, ad)
		}
		if err := w.WriteMsg(response); err != nil {
			return true, dns.RcodeServerFailure, err
		}
		return true, dns.RcodeSuccess, nil
	case <-timer.C:
		return false, 0, nil
	}
}

func (c *Cache) shouldPrefetch(i *item, now time.Time) bool {
	if c.prefetch <= 0 {
		return false
	}
	i.Update(c.duration, now)
	threshold := int(math.Ceil(float64(c.percentage) / 100 * float64(i.origTTL/time.Second)))
	return i.Hits() >= c.prefetch && i.ttl(now) <= threshold
}

func authenticatedRefreshState(state request.Request) request.Request {
	state.Req = state.Req.Copy()
	state.Req.AuthenticatedData = true
	return state
}

// Name implements the Handler interface.
func (c *Cache) Name() string { return "cache" }

// getIfNotStale returns an item if it exists in the cache and has not expired.
func (c *Cache) getIfNotStale(now time.Time, state request.Request, subnet, exactMatch *net.IPNet, srcOrig net.IP, server string) *item {
	cacheRequests.WithLabelValues(server, c.zonesMetricLabel, c.viewMetricLabel).Inc()

	k := hash(state.Name(), state.QType(), state.QClass(), state.Do(), state.Req.CheckingDisabled, &zeroSubnet)

	// ECS, answering from cache
	//
	// 1. If no ECS option was provided, the client's address is used.
	//
	if subnet == nil {
		if i := c.getIfNotStaleInner(k, now, state, srcOrig, server); i != nil {
			return i
		}
	} else {
		if i := c.getIfNotStaleInner(k, now, state, subnet.IP, server); i != nil {
			return i
		}
		//
		// 2.2. If no covering entry is found and SOURCE PREFIX-LENGTH is shorter than the
		// configured maximum length allowed for the cache, repeat the cache
		// lookup for an entry that exactly matches SOURCE PREFIX-LENGTH.
		// These special entries, which do not cover longer prefix lengths,
		// occur as described in the previous section.
		if exactMatch != &zeroSubnet {
			subK := hash(state.Name(), state.QType(), state.QClass(), state.Do(), state.Req.CheckingDisabled, exactMatch)
			if i := c.getIfNotStaleInner(subK, now, state, subnet.IP, server); i != nil {
				return i
			}
		}
	}

	cacheMisses.WithLabelValues(server, c.zonesMetricLabel, c.viewMetricLabel).Inc()
	return nil
}

func (c *Cache) getIfNotStaleInner(k uint64, now time.Time, state request.Request, src net.IP, server string) *item {
	if c.preferPositive && c.staleUpTo > 0 {
		if i, ok := lookup(c.pcache, k, src); ok {
			i = i.answeringItem(state)
			if i != nil {
				if i.withinLifetime(now, c.staleUpTo) {
					cacheHits.WithLabelValues(server, Success, c.zonesMetricLabel, c.viewMetricLabel).Inc()
					return i
				}
			}
		}
	}

	if i, ok := lookup(c.ncache, k, src); ok {
		if i.matches(state) && i.withinLifetime(now, c.staleUpTo) {
			// SERVFAIL is transient; prefer a valid positive cache entry if one
			// exists, so a cached SERVFAIL does not shadow a previously good answer.
			if i.Rcode == dns.RcodeServerFailure {
				if p, pok := lookup(c.pcache, k, src); pok {
					if p.matches(state) && p.withinLifetime(now, c.staleUpTo) {
						cacheHits.WithLabelValues(server, Success, c.zonesMetricLabel, c.viewMetricLabel).Inc()
						return p
					}
				}
			}
			cacheHits.WithLabelValues(server, Denial, c.zonesMetricLabel, c.viewMetricLabel).Inc()
			return i
		}
	}
	if i, ok := lookup(c.pcache, k, src); ok {
		if i.matches(state) && i.withinLifetime(now, c.staleUpTo) {
			cacheHits.WithLabelValues(server, Success, c.zonesMetricLabel, c.viewMetricLabel).Inc()
			return i
		}
	}
	return nil
}

// exists unconditionally returns an item if it exists in the cache.
func (c *Cache) exists(name string, qtype, qclass uint16, do, cd bool, subnet, exactMatch *net.IPNet, srcOrig net.IP) *item {
	k := hash(name, qtype, qclass, do, cd, &zeroSubnet)
	if subnet == nil {
		return c.existsInner(k, srcOrig)
	}
	if i := c.existsInner(k, subnet.IP); i != nil {
		return i
	}
	if exactMatch != &zeroSubnet {
		return c.existsInner(hash(name, qtype, qclass, do, cd, exactMatch), subnet.IP)
	}
	return nil
}

func (c *Cache) existsInner(k uint64, src net.IP) *item {
	if i, ok := lookup(c.ncache, k, src); ok {
		return i
	}
	if i, ok := lookup(c.pcache, k, src); ok {
		return i
	}
	return nil
}

// lookup returns the item stored under key k whose subnet covers src.
func lookup(c *cache.Cache[*iptree.Tree], k uint64, src net.IP) (*item, bool) {
	tree, ok := c.Get(k)
	if !ok {
		return nil, false
	}
	i, ok := tree.GetByIP(src)
	if !ok {
		return nil, false
	}
	return i.(*item), true
}
