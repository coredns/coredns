package cache

import (
	"context"
	"math"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

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

	i := c.getIfNotStale(now, state, server)
	if i == nil {
		crr := &ResponseWriter{ResponseWriter: w, Cache: c, state: state, server: server, do: do, ad: ad, cd: cd,
			nexcept: c.nexcept, pexcept: c.pexcept, wildcardFunc: wildcardFunc(ctx)}
		return c.doRefresh(ctx, state, crr)
	}
	ttl := i.ttl(now)
	if ttl < 0 {
		// serve stale behavior
		if c.verifyStale {
			crr := &ResponseWriter{ResponseWriter: w, Cache: c, state: state, server: server, do: do, cd: cd}
			if c.verifyStaleTimeout > 0 {
				// Background verify: cache the response but do not write to the wire.
				// On timeout, we serve the stale entry below and let the goroutine continue.
				crr.prefetch = true
			}
			cw := newVerifyStaleResponseWriter(crr)
			if c.verifyStaleTimeout == 0 {
				ret, err := c.doRefresh(ctx, state, cw)
				if cw.refreshed {
					return ret, err
				}
			} else if served, ret, err := c.verifyWithTimeout(ctx, state, w, cw, r, do, ad); served {
				return ret, err
			}
		}

		// Adjust the time to get a 0 TTL in the reply built from a stale item.
		now = now.Add(time.Duration(ttl) * time.Second)
		if !c.verifyStale {
			c.tryPrefetch(ctx, i, server, rc, do, cd, now)
		}
		servedStale.WithLabelValues(server, c.zonesMetricLabel, c.viewMetricLabel).Inc()
	} else if c.shouldPrefetch(i, now) {
		c.tryPrefetch(ctx, i, server, rc, do, cd, now)
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

// tryPrefetch dispatches a background prefetch for i if one is not already in
// flight. The CAS on i.refreshing ensures at most one prefetch goroutine per
// item, so prefetch load scales with distinct stale keys rather than QPS.
func (c *Cache) tryPrefetch(ctx context.Context, i *item, server string, req *dns.Msg, do, cd bool, now time.Time) {
	if !i.refreshing.CompareAndSwap(false, true) {
		return
	}
	cw := newPrefetchResponseWriter(server, req, do, cd, c)
	go func() {
		defer i.refreshing.Store(false)
		c.doPrefetch(ctx, cw, i, now)
	}()
}

func (c *Cache) doPrefetch(ctx context.Context, cw *ResponseWriter, i *item, now time.Time) {
	// Use a fresh metadata map to avoid concurrent writes to the original request's metadata.
	ctx = metadata.ContextWithMetadata(ctx)
	cachePrefetches.WithLabelValues(cw.server, c.zonesMetricLabel, c.viewMetricLabel).Inc()
	c.doRefresh(ctx, cw.state, cw)

	// When prefetching we loose the item i, and with it the frequency
	// that we've gathered sofar. See we copy the frequencies info back
	// into the new item that was stored in the cache.
	if i1 := c.exists(cw.state.Name(), cw.state.QType(), cw.do, cw.cd); i1 != nil {
		i1.Reset(now, i.Hits())
	}
}

func (c *Cache) doRefresh(ctx context.Context, state request.Request, cw dns.ResponseWriter) (int, error) {
	return plugin.NextOrFailure(c.Name(), c.Next, ctx, cw, state.Req)
}

// verifyWithTimeout runs the upstream verify in a background goroutine and races it
// against verifyStaleTimeout. If the verify completes within the timeout and the
// response is cacheable (NoError or NXDomain), the freshly cached entry is served
// to the client and served is true. Otherwise served is false and the caller falls
// through to serve stale; the goroutine continues to run and any successful response
// will update the cache without writing to the (now-detached) client connection.
func (c *Cache) verifyWithTimeout(ctx context.Context, state request.Request, w dns.ResponseWriter, cw *verifyStaleResponseWriter, r *dns.Msg, do, ad bool) (served bool, code int, err error) {
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		rc, re := c.doRefresh(ctx, state, cw)
		done <- result{rc, re}
	}()
	timer := time.NewTimer(c.verifyStaleTimeout)
	defer timer.Stop()
	select {
	case res := <-done:
		if !cw.refreshed {
			return false, 0, nil
		}
		fresh := c.exists(state.Name(), state.QType(), state.Do(), state.Req.CheckingDisabled)
		if fresh == nil {
			// Should not happen: refreshed=true means the upstream response was cacheable.
			return true, res.code, res.err
		}
		now := c.now().UTC()
		if c.keepttl {
			now = fresh.stored
		}
		resp := fresh.toMsg(r, now, do, ad)
		if err := w.WriteMsg(resp); err != nil {
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
	threshold := int(math.Ceil(float64(c.percentage) / 100 * float64(i.origTTL)))
	return i.Hits() >= c.prefetch && i.ttl(now) <= threshold
}

// Name implements the Handler interface.
func (c *Cache) Name() string { return "cache" }

// getIfNotStale returns an item if it exists in the cache and has not expired.
func (c *Cache) getIfNotStale(now time.Time, state request.Request, server string) *item {
	k := hash(state.Name(), state.QType(), state.Do(), state.Req.CheckingDisabled)
	cacheRequests.WithLabelValues(server, c.zonesMetricLabel, c.viewMetricLabel).Inc()

	if i, ok := c.ncache.Get(k); ok {
		ttl := i.ttl(now)
		if i.matches(state) && (ttl > 0 || (c.staleUpTo > 0 && -ttl < int(c.staleUpTo.Seconds()))) {
			cacheHits.WithLabelValues(server, Denial, c.zonesMetricLabel, c.viewMetricLabel).Inc()
			return i
		}
	}
	if i, ok := c.pcache.Get(k); ok {
		ttl := i.ttl(now)
		if i.matches(state) && (ttl > 0 || (c.staleUpTo > 0 && -ttl < int(c.staleUpTo.Seconds()))) {
			cacheHits.WithLabelValues(server, Success, c.zonesMetricLabel, c.viewMetricLabel).Inc()
			return i
		}
	}
	cacheMisses.WithLabelValues(server, c.zonesMetricLabel, c.viewMetricLabel).Inc()
	return nil
}

// exists unconditionally returns an item if it exists in the cache.
func (c *Cache) exists(name string, qtype uint16, do, cd bool) *item {
	k := hash(name, qtype, do, cd)
	if i, ok := c.ncache.Get(k); ok {
		return i
	}
	if i, ok := c.pcache.Get(k); ok {
		return i
	}
	return nil
}
