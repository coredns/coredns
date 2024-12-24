// Package cache implements a cache.
package cache

import (
	"hash/fnv"
	"net"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/cache"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/request"
	"github.com/infobloxopen/go-trees/iptree"

	"github.com/miekg/dns"
)

// Cache is a plugin that looks up responses in a cache and caches replies.
// It has a success and a denial of existence cache.
type Cache struct {
	Next  plugin.Handler
	Zones []string

	zonesMetricLabel string
	viewMetricLabel  string

	ncache  *cache.Cache
	ncap    int
	nttl    time.Duration
	minnttl time.Duration

	pcache  *cache.Cache
	pcap    int
	pttl    time.Duration
	minpttl time.Duration
	failttl time.Duration // TTL for caching SERVFAIL responses

	mask_v4_size uint8
	mask_v6_size uint8

	// Prefetch.
	prefetch   int
	duration   time.Duration
	percentage int

	// Stale serve
	staleUpTo   time.Duration
	verifyStale bool

	// Positive/negative zone exceptions
	pexcept []string
	nexcept []string

	// Keep ttl option
	keepttl bool

	// Testing.
	now func() time.Time
}

// New returns an initialized Cache with default settings. It's up to the
// caller to set the Next handler.
func New() *Cache {
	return &Cache{
		Zones:        []string{"."},
		pcap:         defaultCap,
		pcache:       cache.New(defaultCap),
		pttl:         maxTTL,
		minpttl:      minTTL,
		ncap:         defaultCap,
		ncache:       cache.New(defaultCap),
		nttl:         maxNTTL,
		minnttl:      minNTTL,
		failttl:      minNTTL,
		prefetch:     0,
		duration:     1 * time.Minute,
		percentage:   10,
		now:          time.Now,
		mask_v4_size: 32,
		mask_v6_size: 128,
	}
}

// key returns key under which we store the item, -1 will be returned if we don't store the message.
// Currently we do not cache Truncated, errors zone transfers or dynamic update messages.
// qname holds the already lowercased qname.
func key(qname string, exactMatch *net.IPNet, m *dns.Msg, t response.Type, do, cd bool) (bool, uint64) {
	// We don't store truncated responses.
	if m.Truncated {
		return false, 0
	}
	// Nor errors or Meta or Update.
	if t == response.OtherError || t == response.Meta || t == response.Update {
		return false, 0
	}

	return true, hash(qname, m.Question[0].Qtype, do, cd, exactMatch)
}

var one = []byte("1")
var zero = []byte("0")

func hash(qname string, qtype uint16, do, cd bool, exactMatch *net.IPNet) uint64 {
	h := fnv.New64()

	if do {
		h.Write(one)
	} else {
		h.Write(zero)
	}

	if cd {
		h.Write(one)
	} else {
		h.Write(zero)
	}

	h.Write(exactMatch.IP)
	h.Write(exactMatch.Mask)
	h.Write([]byte{byte(qtype >> 8)})
	h.Write([]byte{byte(qtype)})
	h.Write([]byte(qname))
	return h.Sum64()
}

func computeTTL(msgTTL, minTTL, maxTTL time.Duration) time.Duration {
	ttl := msgTTL
	if ttl < minTTL {
		ttl = minTTL
	}
	if ttl > maxTTL {
		ttl = maxTTL
	}
	return ttl
}

// ResponseWriter is a response writer that caches the reply message.
type ResponseWriter struct {
	dns.ResponseWriter
	*Cache
	state      request.Request
	server     string // Server handling the request.
	subnet     *net.IPNet
	exactMatch *net.IPNet
	ecs        *dns.EDNS0_SUBNET

	do         bool // When true the original request had the DO bit set.
	cd         bool // When true the original request had the CD bit set.
	ad         bool // When true the original request had the AD bit set.
	prefetch   bool // When true write nothing back to the client.
	remoteAddr net.Addr

	wildcardFunc func() string // function to retrieve wildcard name that synthesized the result.

	pexcept []string // positive zone exceptions
	nexcept []string // negative zone exceptions
}

// newPrefetchResponseWriter returns a Cache ResponseWriter to be used in
// prefetch requests. It ensures RemoteAddr() can be called even after the
// original connection has already been closed.
func newPrefetchResponseWriter(server string, state request.Request, subnet *net.IPNet, exactMatch *net.IPNet, ecs *dns.EDNS0_SUBNET, c *Cache) *ResponseWriter {
	// Resolve the address now, the connection might be already closed when the
	// actual prefetch request is made.
	addr := state.W.RemoteAddr()
	// The protocol of the client triggering a cache prefetch doesn't matter.
	// The address type is used by request.Proto to determine the response size,
	// and using TCP ensures the message isn't unnecessarily truncated.
	if u, ok := addr.(*net.UDPAddr); ok {
		addr = &net.TCPAddr{IP: u.IP, Port: u.Port, Zone: u.Zone}
	}

	return &ResponseWriter{
		ResponseWriter: state.W,
		Cache:          c,
		state:          state,
		server:         server,
		do:             state.Do(),
		cd:             state.Req.CheckingDisabled,
		prefetch:       true,
		remoteAddr:     addr,
		subnet:         subnet,
		exactMatch:     exactMatch,
		ecs:            ecs,
	}
}

// RemoteAddr implements the dns.ResponseWriter interface.
func (w *ResponseWriter) RemoteAddr() net.Addr {
	if w.remoteAddr != nil {
		return w.remoteAddr
	}
	return w.ResponseWriter.RemoteAddr()
}

// WriteMsg implements the dns.ResponseWriter interface.
func (w *ResponseWriter) WriteMsg(res *dns.Msg) error {
	o := res.IsEdns0()
	subnet := w.subnet
	hadEcs := false
	if o != nil {
		for _, s := range o.Option {
			if ecs, ok := s.(*dns.EDNS0_SUBNET); ok {
				hadEcs = true

				// https://www.rfc-editor.org/rfc/rfc7871#section-7.3
				// If FAMILY, SOURCE PREFIX-LENGTH, and SOURCE PREFIX-LENGTH bits of
				// ADDRESS in the response don't match the non-zero fields in the
				// corresponding query, the full response MUST be dropped.

				// If the query had no ECS, drop:
				// the RFC doesn't explicitly require this,
				// but it seems like the correct behavior.
				if w.ecs == nil {
					return nil
				}

				if ecs.Family != w.ecs.Family {
					return nil
				}

				// This part is weird: https://www.rfc-editor.org/rfc/rfc7871#section-11 says that
				// "the ECS option in a response packet MUST contain the
				// full FAMILY, ADDRESS, and SOURCE PREFIX-LENGTH fields from the
				// corresponding query"
				//
				// Which means that if there is a mismatch in the source netmask, we must drop;
				//
				// but https://www.rfc-editor.org/rfc/rfc7871#section-7.3 says
				// "If FAMILY, SOURCE PREFIX-LENGTH, and SOURCE PREFIX-LENGTH bits of
				// ADDRESS in the response don't match the non-zero fields in the
				// corresponding query, the full response MUST be dropped."
				//
				// Which implies that if the source netmask is 0, comparison should be skipped;
				//
				// And also
				// "In a response to a query that specified only SOURCE
				// PREFIX-LENGTH for privacy masking, the FAMILY and ADDRESS fields MUST
				// contain the appropriate non-zero information that the Authoritative
				// Nameserver used to generate the answer, so that it can be cached
				// accordingly."
				//
				// Which also implies that requests with a 0 source prefix may return a non-zero address...
				//
				// I choose to be safe, respecting section 11 and dropping all requests with non-matching
				// source prefix and address, regardless of the mask.
				if ecs.SourceNetmask != w.ecs.SourceNetmask {
					return nil
				}
				if !ecs.Address.Equal(w.ecs.Address) {
					return nil
				}

				// Records that are cached as /0 because of a query's SOURCE PREFIX-
				// LENGTH of 0 MUST be distinguished from those that are cached as /0
				// because of a response's SCOPE PREFIX-LENGTH of 0.  The former should
				// only be used for other /0 queries that the Intermediate Resolver
				// receives, but the latter is suitable as a response for all networks.
				if w.ecs.SourceNetmask == 0 {
					subnet = &privateZeroSubnet
				}

				// If SCOPE PREFIX-LENGTH is not longer than SOURCE PREFIX-LENGTH, store
				// SCOPE PREFIX-LENGTH bits of ADDRESS, and then mark the response as
				// valid for all addresses that fall within that range.
				if ecs.SourceScope < ecs.SourceNetmask { // The ecs.SourceScope == ecs.SourceNetmask case is handled by default
					// req 10.0.0.0/24, resp valid for 10.0.0.0/8

					if ecs.SourceScope == 0 {
						subnet = &zeroSubnet
						break
					}
					var mask net.IPMask
					if ecs.Family == 1 {
						mask = net.CIDRMask(int(ecs.SourceScope), 32)
					} else {
						mask = net.CIDRMask(int(ecs.SourceScope), 128)
					}
					subnet = &net.IPNet{
						IP:   subnet.IP.Mask(mask),
						Mask: mask,
					}
				} else if ecs.SourceScope > ecs.SourceNetmask {
					// req 10.0.0.0/8, resp valid only for 10.0.0.0/24 (and not i.e. 10.0.0.1/24, which is in 10.0.0.0/8)

					// A SCOPE PREFIX-LENGTH value longer than SOURCE PREFIX-LENGTH
					// indicates that the provided prefix length was not specific enough to
					// select the most appropriate Tailored Response.  Future queries for
					// the name within the specified network SHOULD use the longer SCOPE
					// PREFIX-LENGTH.  Factors affecting whether the Recursive Resolver
					// would use the longer length include the amount of privacy masking the
					// operator wants to provide their users, and the additional resource
					// implications for the cache.
					//
					// If an Intermediate Nameserver receives a response that has a longer
					// SCOPE PREFIX-LENGTH than SOURCE PREFIX-LENGTH that it provided in its
					// query, it SHOULD still provide the result as the answer to the
					// triggering client request even if the client is in a different
					// address range.
					//
					//
					// TODO: The Intermediate Nameserver MAY instead opt to retry
					// with a longer SOURCE PREFIX-LENGTH to get a better reply before
					// responding to its client, as long as it does not exceed a SOURCE
					// PREFIX-LENGTH specified in the query that triggered resolution, but
					// this obviously has implications for the latency of the overall
					// lookup.

					// Cache implications:

					// Similarly, if SOURCE PREFIX-LENGTH is the maximum configured for the
					// cache, store SOURCE PREFIX-LENGTH bits of ADDRESS, and then mark the
					// response as valid for all addresses that fall within that range.
					//
					// Weirdly, this means to cache by the requested prefix, instead of the returned one.
					// i.e. req: 10.0.0.0/8 (max is 8), response covers only 10.0.0.0/24, cache for 10.0.0.0/8
					//
					// Implemented by default (subnet = w.subnet)

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
					if w.exactMatch != &zeroSubnet {
						subnet = w.exactMatch
					}
				}

				break
			}
		}
	}

	// If no ECS option is contained in the response, the Intermediate
	// Nameserver SHOULD treat this as being equivalent to having received a
	// SCOPE PREFIX-LENGTH of 0
	if !hadEcs {
		subnet = &zeroSubnet
	}

	mt, _ := response.Typify(res, w.now().UTC())

	// key returns empty string for anything we don't want to cache.
	hasKey, key := key(w.state.Name(), w.exactMatch, res, mt, w.do, w.cd)

	msgTTL := dnsutil.MinimalTTL(res, mt)
	var duration time.Duration
	if mt == response.NameError || mt == response.NoData {
		duration = computeTTL(msgTTL, w.minnttl, w.nttl)
	} else if mt == response.ServerError {
		duration = w.failttl
	} else {
		duration = computeTTL(msgTTL, w.minpttl, w.pttl)
	}

	if hasKey && duration > 0 {
		if w.state.Match(res) {
			w.set(res, key, mt, subnet, duration)
			cacheSize.WithLabelValues(w.server, Success, w.zonesMetricLabel, w.viewMetricLabel).Set(float64(w.pcache.Len()))
			cacheSize.WithLabelValues(w.server, Denial, w.zonesMetricLabel, w.viewMetricLabel).Set(float64(w.ncache.Len()))
		} else {
			// Don't log it, but increment counter
			cacheDrops.WithLabelValues(w.server, w.zonesMetricLabel, w.viewMetricLabel).Inc()
		}
	}

	if w.prefetch {
		return nil
	}

	// Apply capped TTL to this reply to avoid jarring TTL experience 1799 -> 8 (e.g.)
	ttl := uint32(duration.Seconds())
	res.Answer = filterRRSlice(res.Answer, ttl, false)
	res.Ns = filterRRSlice(res.Ns, ttl, false)
	res.Extra = filterRRSlice(res.Extra, ttl, false)

	if !w.do && !w.ad {
		// unset AD bit if requester is not OK with DNSSEC
		// But retain AD bit if requester set the AD bit in the request, per RFC6840 5.7-5.8
		res.AuthenticatedData = false
	}

	return w.ResponseWriter.WriteMsg(res)
}

func (w *ResponseWriter) set(m *dns.Msg, key uint64, mt response.Type, subnet *net.IPNet, duration time.Duration) {
	// duration is expected > 0
	// and key is valid
	switch mt {
	case response.NoError, response.Delegation:
		if plugin.Zones(w.pexcept).Matches(m.Question[0].Name) != "" {
			// zone is in exception list, do not cache
			return
		}
		var tree *iptree.Tree
		if _tree, ok := w.pcache.Get(key); ok {
			tree = _tree.(*iptree.Tree)
		} else {
			tree = iptree.NewTree()
		}

		i := newItem(m, w.now(), duration)
		if w.wildcardFunc != nil {
			i.wildcard = w.wildcardFunc()
		}
		if w.pcache.Add(key, tree.InsertNet(subnet, i)) {
			evictions.WithLabelValues(w.server, Success, w.zonesMetricLabel, w.viewMetricLabel).Inc()
		}
		// when pre-fetching, remove the negative cache entry if it exists
		if w.prefetch {
			if _tree, ok := w.ncache.Get(key); ok {
				tree = _tree.(*iptree.Tree)
				if tree, ok = tree.DeleteByNet(subnet); ok {
					w.ncache.Add(key, tree)
				}
			}
		}

	case response.NameError, response.NoData, response.ServerError:
		if plugin.Zones(w.nexcept).Matches(m.Question[0].Name) != "" {
			// zone is in exception list, do not cache
			return
		}
		var tree *iptree.Tree
		if _tree, ok := w.ncache.Get(key); ok {
			tree = _tree.(*iptree.Tree)
		} else {
			tree = iptree.NewTree()
		}
		i := newItem(m, w.now(), duration)
		if w.wildcardFunc != nil {
			i.wildcard = w.wildcardFunc()
		}
		if w.ncache.Add(key, tree.InsertNet(subnet, i)) {
			evictions.WithLabelValues(w.server, Denial, w.zonesMetricLabel, w.viewMetricLabel).Inc()
		}

	case response.OtherError:
		// don't cache these
	default:
		log.Warningf("Caching called with unknown classification: %d", mt)
	}
}

// Write implements the dns.ResponseWriter interface.
func (w *ResponseWriter) Write(buf []byte) (int, error) {
	log.Warning("Caching called with Write: not caching reply")
	if w.prefetch {
		return 0, nil
	}
	n, err := w.ResponseWriter.Write(buf)
	return n, err
}

// verifyStaleResponseWriter is a response writer that only writes messages if they should replace a
// stale cache entry, and otherwise discards them.
type verifyStaleResponseWriter struct {
	*ResponseWriter
	refreshed bool // set to true if the last WriteMsg wrote to ResponseWriter, false otherwise.
}

// newVerifyStaleResponseWriter returns a ResponseWriter to be used when verifying stale cache
// entries. It only forward writes if an entry was successfully refreshed according to RFC8767,
// section 4 (response is NoError or NXDomain), and ignores any other response.
func newVerifyStaleResponseWriter(w *ResponseWriter) *verifyStaleResponseWriter {
	return &verifyStaleResponseWriter{
		w,
		false,
	}
}

// WriteMsg implements the dns.ResponseWriter interface.
func (w *verifyStaleResponseWriter) WriteMsg(res *dns.Msg) error {
	w.refreshed = false
	if res.Rcode == dns.RcodeSuccess || res.Rcode == dns.RcodeNameError {
		w.refreshed = true
		return w.ResponseWriter.WriteMsg(res) // stores to the cache and send to client
	}
	return nil // else discard
}

const (
	maxTTL  = dnsutil.MaximumDefaulTTL
	minTTL  = dnsutil.MinimalDefaultTTL
	maxNTTL = dnsutil.MaximumDefaulTTL / 2
	minNTTL = dnsutil.MinimalDefaultTTL

	defaultCap = 10000 // default capacity of the cache.

	// Success is the class for caching positive caching.
	Success = "success"
	// Denial is the class defined for negative caching.
	Denial = "denial"
)
