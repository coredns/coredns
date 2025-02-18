package cache

import (
	"context"
	"math"
	"net"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/metrics"
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
	rc := r.Copy() // We potentially modify r, to prevent other plugins from seeing this (r is a pointer), copy r into rc.
	state := request.Request{W: w, Req: rc}
	do := state.Do()
	cd := r.CheckingDisabled
	ad := r.AuthenticatedData

	zone := plugin.Zones(c.Zones).Matches(state.Name())
	if zone == "" {
		return plugin.NextOrFailure(c.Name(), c.Next, ctx, w, rc)
	}

	now := c.now().UTC()
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

	ttl := 0
	i := c.getIgnoreTTL(now, state, subnet, exactMatch, srcOrig, server, false)
	if i == nil {
		crr := &ResponseWriter{ResponseWriter: w, Cache: c, state: state, server: server, do: do, ad: ad, cd: cd,
			subnet:     subnet,
			exactMatch: exactMatch,
			ecs:        ecs,
			nexcept:    c.nexcept, pexcept: c.pexcept, wildcardFunc: wildcardFunc(ctx)}
		return c.doRefresh(ctx, state, crr)
	}
	ttl = i.ttl(now)
	if ttl < 0 {
		// serve stale behavior
		if c.verifyStale {
			crr := &ResponseWriter{ResponseWriter: w, Cache: c, state: state, server: server, do: do, cd: cd,
				subnet:     subnet,
				exactMatch: exactMatch,
				ecs:        ecs}
			cw := newVerifyStaleResponseWriter(crr)
			ret, err := c.doRefresh(ctx, state, cw)
			if cw.refreshed {
				return ret, err
			}
		}

		// Adjust the time to get a 0 TTL in the reply built from a stale item.
		now = now.Add(time.Duration(ttl) * time.Second)
		if !c.verifyStale {
			cw := newPrefetchResponseWriter(server, state, subnet, exactMatch, ecs, c)
			go c.doPrefetch(ctx, state, cw, i, subnet, exactMatch, srcOrig, now, server)
		}
		servedStale.WithLabelValues(server, c.zonesMetricLabel, c.viewMetricLabel).Inc()
	} else if c.shouldPrefetch(i, now) {
		cw := newPrefetchResponseWriter(server, state, subnet, exactMatch, ecs, c)
		go c.doPrefetch(ctx, state, cw, i, subnet, exactMatch, srcOrig, now, server)
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
	resp := i.toMsg(r, now, do, ad)
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

func (c *Cache) doPrefetch(ctx context.Context, state request.Request, cw *ResponseWriter, i *item, subnet *net.IPNet, exactMatch *net.IPNet, srcOrig net.IP, now time.Time, server string) {
	cachePrefetches.WithLabelValues(cw.server, c.zonesMetricLabel, c.viewMetricLabel).Inc()
	c.doRefresh(ctx, state, cw)

	// When prefetching we loose the item i, and with it the frequency
	// that we've gathered sofar. See we copy the frequencies info back
	// into the new item that was stored in the cache.
	if i1 := c.getIgnoreTTL(now, state, subnet, exactMatch, srcOrig, server, true); i1 != nil {
		i1.Freq.Reset(now, i.Freq.Hits())
	}
}

func (c *Cache) doRefresh(ctx context.Context, state request.Request, cw dns.ResponseWriter) (int, error) {
	return plugin.NextOrFailure(c.Name(), c.Next, ctx, cw, state.Req)
}

func (c *Cache) shouldPrefetch(i *item, now time.Time) bool {
	if c.prefetch <= 0 {
		return false
	}
	i.Freq.Update(c.duration, now)
	threshold := int(math.Ceil(float64(c.percentage) / 100 * float64(i.origTTL)))
	return i.Freq.Hits() >= c.prefetch && i.ttl(now) <= threshold
}

// Name implements the Handler interface.
func (c *Cache) Name() string { return "cache" }

// getIgnoreTTL unconditionally returns an item if it exists in the cache.
func (c *Cache) getIgnoreTTL(now time.Time, state request.Request, subnet *net.IPNet, exactMatch *net.IPNet, srcOrig net.IP, server string, justCheckExists bool) *item {
	cacheRequests.WithLabelValues(server, c.zonesMetricLabel, c.viewMetricLabel).Inc()

	k := hash(state.Name(), state.QType(), state.Do(), state.Req.CheckingDisabled, &zeroSubnet)

	// ECS, answering from cache
	//
	// 1. If no ECS option was provided, the client's address is used.
	//
	if subnet == nil {
		i := c.getIgnoreTTLInner(k, now, state, srcOrig, server, justCheckExists)
		if i != nil {
			return i
		}
	} else {
		i := c.getIgnoreTTLInner(k, now, state, subnet.IP, server, justCheckExists)
		if i != nil {
			return i
		}
		//
		// 2.2. If no covering entry is found and SOURCE PREFIX-LENGTH is shorter than the
		// configured maximum length allowed for the cache, repeat the cache
		// lookup for an entry that exactly matches SOURCE PREFIX-LENGTH.
		// These special entries, which do not cover longer prefix lengths,
		// occur as described in the previous section.

		if exactMatch != &zeroSubnet {
			subK := hash(state.Name(), state.QType(), state.Do(), state.Req.CheckingDisabled, exactMatch)
			i := c.getIgnoreTTLInner(subK, now, state, subnet.IP, server, justCheckExists)
			if i != nil {
				return i
			}
		}
	}

	cacheMisses.WithLabelValues(server, c.zonesMetricLabel, c.viewMetricLabel).Inc()
	return nil
}

func (c *Cache) getIgnoreTTLInner(k uint64, now time.Time, state request.Request, src net.IP, server string, justCheckExists bool) *item {
	if i, ok := c.ncache.Get(k); ok {
		tree := i.(*iptree.Tree)
		if ii, ok := tree.GetByIP(src); ok {
			itm := ii.(*item)
			if justCheckExists {
				return itm
			}
			ttl := itm.ttl(now)
			if itm.matches(state) && (ttl > 0 || (c.staleUpTo > 0 && -ttl < int(c.staleUpTo.Seconds()))) {
				cacheHits.WithLabelValues(server, Denial, c.zonesMetricLabel, c.viewMetricLabel).Inc()
				return itm
			}
		}
	}
	if i, ok := c.pcache.Get(k); ok {
		tree := i.(*iptree.Tree)
		if ii, ok := tree.GetByIP(src); ok {
			itm := ii.(*item)
			if justCheckExists {
				return itm
			}
			ttl := itm.ttl(now)
			if itm.matches(state) && (ttl > 0 || (c.staleUpTo > 0 && -ttl < int(c.staleUpTo.Seconds()))) {
				cacheHits.WithLabelValues(server, Success, c.zonesMetricLabel, c.viewMetricLabel).Inc()
				return itm
			}
		}
	}

	return nil
}
