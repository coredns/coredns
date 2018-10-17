package cache

import (
	"context"
	"math"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// ServeDNS implements the plugin.Handler interface.
func (c *Cache) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	zone := plugin.Zones(c.Zones).Matches(state.Name())
	if zone == "" {
		return plugin.NextOrFailure(c.Name(), c.Next, ctx, w, r)
	}

	now := c.now().UTC()

	server := metrics.WithServer(ctx)

	i, found := c.get(now, state, server)
	if i != nil && found {
		resp := i.toMsg(r, now)

		w.WriteMsg(resp)

		if c.prefetch > 0 {
			ttl := i.ttl(now)
			i.Freq.Update(c.duration, now)

			threshold := int(math.Ceil(float64(c.percentage) / 100 * float64(i.origTTL)))
			if i.Freq.Hits() >= c.prefetch && ttl <= threshold {
				cw := newPrefetchResponseWriter(server, state, c)
				go func(w dns.ResponseWriter) {
					c.metric.Prefetches.WithLabelValues(server).Inc()
					plugin.NextOrFailure(c.Name(), c.Next, ctx, w, r)

					// When prefetching we loose the item i, and with it the frequency
					// that we've gathered sofar. See we copy the frequencies info back
					// into the new item that was stored in the cache.
					if i1 := c.exists(state); i1 != nil {
						i1.Freq.Reset(now, i.Freq.Hits())
					}
				}(cw)
			}
		}
		return dns.RcodeSuccess, nil
	}

	crr := &ResponseWriter{ResponseWriter: w, Cache: c, state: state, server: server}
	return plugin.NextOrFailure(c.Name(), c.Next, ctx, crr, r)
}

// Name implements the Handler interface.
func (c *Cache) Name() string { return "cache" }

func (c *Cache) get(now time.Time, state request.Request, server string) (*item, bool) {
	k := hash(state.Name(), state.QType(), state.Do())

	if i, ok := c.ncache.Get(k); ok && i.(*item).ttl(now) > 0 {
		c.metric.Hits.WithLabelValues(server, Denial).Inc()
		return i.(*item), true
	}

	if i, ok := c.pcache.Get(k); ok && i.(*item).ttl(now) > 0 {
		c.metric.Hits.WithLabelValues(server, Success).Inc()
		return i.(*item), true
	}
	c.metric.Hits.WithLabelValues(server).Inc()
	return nil, false
}

func (c *Cache) exists(state request.Request) *item {
	k := hash(state.Name(), state.QType(), state.Do())
	if i, ok := c.ncache.Get(k); ok {
		return i.(*item)
	}
	if i, ok := c.pcache.Get(k); ok {
		return i.(*item)
	}
	return nil
}