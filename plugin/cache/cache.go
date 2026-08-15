// Package cache implements a cache.
package cache

import (
	"encoding/binary"
	"hash/fnv"
	"net"
	"strings"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/cache"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Cache is a plugin that looks up responses in a cache and caches replies.
// It has a success and a denial of existence cache.
type Cache struct {
	Next  plugin.Handler
	Zones []string

	zonesMetricLabel string
	viewMetricLabel  string

	ncache  *cache.Cache[*item]
	ncap    int
	nttl    time.Duration
	minnttl time.Duration

	pcache  *cache.Cache[*item]
	pcap    int
	pttl    time.Duration
	minpttl time.Duration
	failttl time.Duration // TTL for caching SERVFAIL responses

	// Prefetch.
	prefetch   int
	duration   time.Duration
	percentage int

	// Stale serve
	staleUpTo          time.Duration
	verifyStale        bool
	verifyStaleTimeout time.Duration // 0 means wait for upstream until its own timeout (current default).
	preferPositive     bool
	staleTTL           time.Duration // TTL returned with stale responses; 0 preserves the legacy behavior.
	staleRecheck       time.Duration // Delay after a failed refresh before another attempt; 0 preserves the legacy behavior.

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
		Zones:      []string{"."},
		pcap:       defaultCap,
		pcache:     cache.New[*item](defaultCap),
		pttl:       maxTTL,
		minpttl:    minTTL,
		ncap:       defaultCap,
		ncache:     cache.New[*item](defaultCap),
		nttl:       maxNTTL,
		minnttl:    minNTTL,
		failttl:    minNTTL,
		prefetch:   0,
		duration:   1 * time.Minute,
		percentage: 10,
		now:        time.Now,
	}
}

// key returns key under which we store the item, -1 will be returned if we don't store the message.
// Currently we do not cache Truncated, errors zone transfers or dynamic update messages.
// qname holds the already lowercased qname.
func key(qname string, m *dns.Msg, t response.Type, do, cd bool) (bool, uint64) {
	// We don't store truncated responses.
	if m.Truncated {
		return false, 0
	}
	// Nor errors or Meta or Update.
	if t == response.OtherError || t == response.Meta || t == response.Update {
		return false, 0
	}
	// Negative caching requires an SOA record to determine the denial TTL.
	if t == response.NameError && !hasSOA(m) {
		return false, 0
	}
	// An upstream may return NOERROR with a non-empty answer that still does not
	// resolve the question and without an SOA to bound a negative TTL: a CNAME
	// chain that does not terminate in the queried type (an incomplete recursion
	// result from a forwarder). This is effectively an SOA-less NODATA response,
	// which per RFC 2308 section 5 SHOULD NOT be cached. response.Typify classifies
	// it as NoError because the answer section is non-empty, so caching it in the
	// positive cache would replay the non-answer to clients until it expires. Skip
	// caching so the next query is resolved upstream again. An empty answer section
	// is deliberately left cacheable: it is indistinguishable from a legitimate
	// NOERROR positive response that carries its data outside the answer section
	// (for example the whoami plugin, which answers in the additional section).
	if t == response.NoError && !hasSOA(m) && isNODATA(m) {
		return false, 0
	}

	return true, hash(qname, m.Question[0].Qtype, m.Question[0].Qclass, do, cd)
}

func hasSOA(m *dns.Msg) bool {
	for _, r := range m.Ns {
		if r.Header().Rrtype == dns.TypeSOA {
			return true
		}
	}
	return false
}

// cacheResponseType returns the response type used by the cache. Typify treats
// any NOERROR response with a non-empty answer section as NoError, but RFC 2308
// NODATA responses may contain a CNAME chain. Reclassify those responses when
// an SOA provides the negative cache TTL.
func cacheResponseType(m *dns.Msg, now time.Time) response.Type {
	t, _ := response.Typify(m, now)
	if t == response.NoError && hasSOA(m) && isNODATA(m) {
		return response.NoData
	}
	return t
}

// answersQuestion reports whether a NOERROR response contains an answer to its
// question. For types other than CNAME and ANY, the queried type must exist at
// the terminal owner reached by following the CNAME chain from QNAME.
func answersQuestion(m *dns.Msg) bool {
	if m == nil || m.Rcode != dns.RcodeSuccess || len(m.Question) == 0 || len(m.Answer) == 0 {
		return false
	}
	q := m.Question[0]
	return answerHasType(m.Answer, q.Name, q.Qtype, q.Qclass)
}

func answerHasType(answer []dns.RR, name string, qtype, qclass uint16) bool {
	if len(answer) == 0 {
		return false
	}
	if qtype == dns.TypeANY {
		for _, r := range answer {
			h := r.Header()
			if classMatches(h.Class, qclass) && strings.EqualFold(h.Name, name) {
				return true
			}
		}
		return false
	}
	if qtype == dns.TypeCNAME {
		target, ok := uniqueCNAMETarget(answer, name, qclass)
		return ok && target != ""
	}
	terminal, ok := canonicalName(answer, name, qclass)
	if !ok {
		return false
	}
	name = terminal
	for _, r := range answer {
		h := r.Header()
		if h.Rrtype == qtype && classMatches(h.Class, qclass) && strings.EqualFold(h.Name, name) {
			return true
		}
	}
	return false
}

func classMatches(rrClass, qclass uint16) bool {
	return qclass == dns.ClassANY || rrClass == qclass
}

// usableAnswer reports whether m is a complete, cache-valid positive response
// that answers its question. It intentionally permits TTL-zero responses:
// they are usable for the current client even though they are not retained.
func usableAnswer(m *dns.Msg, now time.Time) bool {
	if m == nil || m.Truncated || cacheResponseType(m, now) != response.NoError {
		return false
	}
	return answersQuestion(m)
}

// isNODATA reports whether a NOERROR response with a non-empty answer section
// does not answer the question. Following RFC 1034 section 3.6.2 and RFC 2308
// sections 1 and 2.2, a query of any type other than CNAME (and ANY) is
// restarted along the CNAME chain, so the effective owner name is the target at
// the end of the CNAME chain that starts at the question name. The response
// answers the question only if it carries a record of the queried type at that
// terminal name (records at any other owner name are irrelevant, and per RFC
// 1034 a CNAME's owner never co-locates other data). This rule is independent of
// the queried type: an MX, TXT, SRV, etc. chain that does not reach the queried
// type is NODATA just like an A or AAAA one. When the chain is malformed (an
// owner with more than one distinct CNAME target, or a loop) it has no
// well-defined terminal name, so the response is treated as NODATA, which errs
// toward re-querying upstream rather than caching a non-answer. An empty answer
// section returns false so that legitimate positive responses carrying data
// outside the answer section (for example the whoami plugin) remain cacheable.
// An ANY query is answered only by a record at the queried owner and in the
// requested class. Note: a bare DNAME (RFC 6672) without its synthesized CNAME
// is treated as NODATA; standard responses include the synthesized CNAME, which
// the chain walk follows.
func isNODATA(m *dns.Msg) bool {
	if len(m.Answer) == 0 {
		return false
	}
	return !answersQuestion(m)
}

// canonicalName follows the owner-linked CNAME chain in answer starting at name
// and returns the terminal target name together with a validity flag. Records
// whose owner is not on the chain are ignored. The chain is invalid (ok=false)
// when it is not a single unambiguous path to a terminal name: an owner that has
// more than one distinct CNAME target violates RFC 2181 section 10.1 (an alias
// has exactly one canonical name), and a revisited owner is a CNAME loop, which
// RFC 1034 section 3.6.2 says must be signalled as an error. Reporting validity
// rather than silently stopping keeps the classification order-independent and
// fail-closed: callers treat a malformed chain as a non-answer. Duplicate CNAME
// records that name the same target are tolerated, since they still describe a
// single canonical name.
func canonicalName(answer []dns.RR, name string, qclass uint16) (string, bool) {
	visited := nameSet{}
	for {
		if visited.contains(name) {
			// Revisited owner: the chain contains a CNAME loop.
			return name, false
		}
		visited.add(name)

		target, ok := uniqueCNAMETarget(answer, name, qclass)
		if !ok {
			// Owner has more than one distinct canonical name.
			return name, false
		}
		if target == "" {
			// Terminal owner reached: no CNAME continues the chain.
			return name, true
		}
		name = target
	}
}

// uniqueCNAMETarget returns the canonical name that owner is aliased to by a
// CNAME record in answer. ok is false when owner carries more than one distinct
// CNAME target, which violates RFC 2181 section 10.1. When owner has no CNAME the
// returned target is empty and ok is true, marking a terminal owner. Duplicate
// CNAME records naming the same target are tolerated.
func uniqueCNAMETarget(answer []dns.RR, owner string, qclass uint16) (target string, ok bool) {
	for _, r := range answer {
		c, isCNAME := r.(*dns.CNAME)
		if !isCNAME || !classMatches(c.Header().Class, qclass) || !strings.EqualFold(c.Header().Name, owner) {
			continue
		}
		if target != "" && !strings.EqualFold(target, c.Target) {
			return "", false
		}
		target = c.Target
	}
	return target, true
}

// nameSet is a set of domain names compared case-insensitively, used to detect
// revisited owners (loops) while walking a CNAME chain.
type nameSet map[string]struct{}

func (s nameSet) contains(name string) bool {
	_, ok := s[strings.ToLower(name)]
	return ok
}

func (s nameSet) add(name string) {
	s[strings.ToLower(name)] = struct{}{}
}

var one = []byte("1")
var zero = []byte("0")

func hash(qname string, qtype, qclass uint16, do, cd bool) uint64 {
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

	var qtypeBytes [2]byte
	binary.BigEndian.PutUint16(qtypeBytes[:], qtype)
	h.Write(qtypeBytes[:])
	var qclassBytes [2]byte
	binary.BigEndian.PutUint16(qclassBytes[:], qclass)
	h.Write(qclassBytes[:])
	h.Write([]byte(qname))
	return h.Sum64()
}

func computeTTL(msgTTL, minTTL, maxTTL time.Duration) time.Duration {
	ttl := min(max(msgTTL, minTTL), maxTTL)
	return ttl
}

// ResponseWriter is a response writer that caches the reply message.
type ResponseWriter struct {
	dns.ResponseWriter
	*Cache
	state  request.Request
	server string // Server handling the request.

	do         bool // When true the original request had the DO bit set.
	cd         bool // When true the original request had the CD bit set.
	ad         bool // When true the original request had the AD bit set.
	prefetch   bool // When true write nothing back to the client.
	remoteAddr net.Addr

	wildcardFunc func() string // function to retrieve wildcard name that synthesized the result.
	lastResponse *dns.Msg      // last response after cache TTL and DNSSEC adjustments.
	lastItem     *item         // cache item written by the last response, if cacheable.

	pexcept []string // positive zone exceptions
	nexcept []string // negative zone exceptions
}

// prefetchAddr is the synthetic remote address for prefetch requests. There is
// no client connection, and per request.Proto the address type is what selects
// the response-size budget; TCP ensures upstream replies aren't truncated.
var prefetchAddr = &net.TCPAddr{}

// newPrefetchResponseWriter returns a ResponseWriter for prefetch requests.
// Prefetch has no client connection: the inner ResponseWriter is nil, WriteMsg
// short-circuits after caching when w.prefetch is true, and the nil-safe
// overrides below make the remaining dns.ResponseWriter methods well-defined.
func newPrefetchResponseWriter(server string, req *dns.Msg, do, cd bool, c *Cache) *ResponseWriter {
	req = req.Copy()
	req.AuthenticatedData = true
	cw := &ResponseWriter{
		Cache:      c,
		server:     server,
		do:         do,
		cd:         cd,
		ad:         true,
		prefetch:   true,
		remoteAddr: prefetchAddr,
	}
	cw.state = request.Request{Req: req}
	return cw
}

// RemoteAddr implements the dns.ResponseWriter interface.
func (w *ResponseWriter) RemoteAddr() net.Addr {
	if w.remoteAddr != nil {
		return w.remoteAddr
	}
	return w.ResponseWriter.RemoteAddr()
}

// The following overrides make a nil inner ResponseWriter well-defined.
// Prefetch constructs a ResponseWriter with no client connection; WriteMsg
// and Write already short-circuit on w.prefetch before delegating, and
// RemoteAddr uses w.remoteAddr. These cover the rest of the interface.

func (w *ResponseWriter) LocalAddr() net.Addr {
	if w.ResponseWriter == nil {
		return prefetchAddr
	}
	return w.ResponseWriter.LocalAddr()
}

func (w *ResponseWriter) Close() error {
	if w.ResponseWriter == nil {
		return nil
	}
	return w.ResponseWriter.Close()
}

func (w *ResponseWriter) TsigStatus() error {
	if w.ResponseWriter == nil {
		return nil
	}
	return w.ResponseWriter.TsigStatus()
}

func (w *ResponseWriter) TsigTimersOnly(b bool) {
	if w.ResponseWriter == nil {
		return
	}
	w.ResponseWriter.TsigTimersOnly(b)
}

func (w *ResponseWriter) Hijack() {
	if w.ResponseWriter == nil {
		return
	}
	w.ResponseWriter.Hijack()
}

// WriteMsg implements the dns.ResponseWriter interface.
func (w *ResponseWriter) WriteMsg(res *dns.Msg) error {
	res = res.Copy()
	w.lastItem = nil
	mt := cacheResponseType(res, w.now().UTC())

	// key returns empty string for anything we don't want to cache.
	hasKey, key := key(w.state.Name(), res, mt, w.do, w.cd)

	var duration time.Duration
	switch mt {
	case response.NameError, response.NoData:
		msgTTL := dnsutil.MinimalTTLWithMaximum(res, mt, w.nttl)
		duration = computeTTL(msgTTL, w.minnttl, w.nttl)
	case response.ServerError:
		duration = w.failttl
	default:
		msgTTL := dnsutil.MinimalTTLWithMaximum(res, mt, w.pttl)
		duration = computeTTL(msgTTL, w.minpttl, w.pttl)
	}

	// Apply capped TTL to this reply to avoid jarring TTL experience 1799 -> 8 (e.g.)
	ttl := uint32(duration.Seconds())
	res.Answer = filterRRSlice(res.Answer, ttl, false)
	res.Ns = filterRRSlice(res.Ns, ttl, false)
	res.Extra = filterRRSlice(res.Extra, ttl, false)

	if hasKey && duration > 0 {
		if w.state.Match(res) {
			w.set(res, key, mt, duration)
			cacheSize.WithLabelValues(w.server, Success, w.zonesMetricLabel, w.viewMetricLabel).Set(float64(w.pcache.Len()))
			cacheSize.WithLabelValues(w.server, Denial, w.zonesMetricLabel, w.viewMetricLabel).Set(float64(w.ncache.Len()))
		} else {
			// Don't log it, but increment counter
			cacheDrops.WithLabelValues(w.server, w.zonesMetricLabel, w.viewMetricLabel).Inc()
		}
	}

	if !w.do && !w.ad {
		// unset AD bit if requester is not OK with DNSSEC
		// But retain AD bit if requester set the AD bit in the request, per RFC6840 5.7-5.8
		res.AuthenticatedData = false
	}
	w.lastResponse = res.Copy()

	if w.prefetch {
		return nil
	}

	return w.ResponseWriter.WriteMsg(res)
}

func (w *ResponseWriter) set(m *dns.Msg, key uint64, mt response.Type, duration time.Duration) {
	// duration is expected > 0
	// and key is valid
	switch mt {
	case response.NoError, response.Delegation:
		if plugin.Zones(w.pexcept).Matches(m.Question[0].Name) != "" {
			// zone is in exception list, do not cache
			return
		}
		i := newItem(m, w.now(), duration)
		if w.wildcardFunc != nil {
			i.wildcard = w.wildcardFunc()
		}
		if w.preferPositive && !i.answering {
			if previous, ok := w.pcache.Get(key); ok {
				i.lastKnownGood = previous.answeringItem(w.state)
			}
		}
		if w.pcache.Add(key, i) {
			evictions.WithLabelValues(w.server, Success, w.zonesMetricLabel, w.viewMetricLabel).Inc()
		}
		w.lastItem = i
		// A positive refresh is the newest state for this key. Under the
		// prefer_positive policy, only remove the denial when this response
		// actually answers the question.
		if (!w.preferPositive && w.prefetch) || (w.preferPositive && i.answering) {
			w.ncache.Remove(key)
		}

	case response.NameError, response.NoData, response.ServerError:
		if plugin.Zones(w.nexcept).Matches(m.Question[0].Name) != "" {
			// zone is in exception list, do not cache
			return
		}
		i := newItem(m, w.now(), duration)
		if w.wildcardFunc != nil {
			i.wildcard = w.wildcardFunc()
		}
		if w.ncache.Add(key, i) {
			evictions.WithLabelValues(w.server, Denial, w.zonesMetricLabel, w.viewMetricLabel).Inc()
		}
		w.lastItem = i

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
	response  *dns.Msg
	item      *item
}

// newVerifyStaleResponseWriter returns a ResponseWriter to be used when verifying stale cache
// entries. It only forwards matching, complete responses that successfully refresh the data
// according to RFC8767, section 4 (response is NoError or NXDomain). With prefer_positive, only
// a usable positive answer is forwarded; other matching responses are cached without being sent
// to the client.
func newVerifyStaleResponseWriter(w *ResponseWriter) *verifyStaleResponseWriter {
	return &verifyStaleResponseWriter{
		ResponseWriter: w,
	}
}

// WriteMsg implements the dns.ResponseWriter interface.
func (w *verifyStaleResponseWriter) WriteMsg(res *dns.Msg) error {
	w.refreshed = false
	w.response = nil
	w.item = nil
	if res == nil || res.Truncated || !w.state.Match(res) {
		return nil
	}
	if w.preferPositive {
		if usableAnswer(res, w.now().UTC()) {
			w.refreshed = true
			err := w.ResponseWriter.WriteMsg(res)
			w.response = w.lastResponse
			w.item = w.lastItem
			return err
		}
		prefetch := w.prefetch
		w.prefetch = true
		err := w.ResponseWriter.WriteMsg(res)
		w.prefetch = prefetch
		return err
	}
	responseType, _ := response.Typify(res, w.now().UTC())
	if responseType == response.OtherError || responseType == response.Meta || responseType == response.Update {
		return nil
	}
	if res.Rcode != dns.RcodeSuccess && res.Rcode != dns.RcodeNameError {
		return nil
	}
	w.refreshed = true
	err := w.ResponseWriter.WriteMsg(res) // stores to the cache and sends to the client
	w.response = w.lastResponse
	w.item = w.lastItem
	return err
}

const (
	maxTTL  = dnsutil.MaximumDefaultTTL
	minTTL  = dnsutil.MinimalDefaultTTL
	maxNTTL = dnsutil.MaximumDefaultTTL / 2
	minNTTL = dnsutil.MinimalDefaultTTL

	defaultCap = 10000 // default capacity of the cache.

	// Success is the class for caching positive caching.
	Success = "success"
	// Denial is the class defined for negative caching.
	Denial = "denial"
)
