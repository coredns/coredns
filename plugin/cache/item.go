package cache

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin/cache/freq"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type item struct {
	Name               string
	QType              uint16
	QClass             uint16
	Rcode              int
	Authoritative      bool
	AuthenticatedData  bool
	RecursionAvailable bool
	Answer             []dns.RR
	Ns                 []dns.RR
	Extra              []dns.RR
	wildcard           string
	do                 bool  // DO used to acquire this response, not the response's DO bit.
	answering          bool  // immutable result of validating that this item answers its question.
	lastKnownGood      *item // answering item retained when a non-answer overwrites this success-cache key.

	origTTL uint32
	stored  time.Time

	*freq.Freq

	*refreshState
}

// refreshState belongs to an acquisition, not to an immutable response holder.
// Holders that only replace lastKnownGood share it so an in-flight refresh's
// completion and failure recheck deadline remain visible to all readers.
type refreshState struct {
	refreshing atomic.Bool
	retryAfter atomic.Pointer[time.Time]
}

func newItem(m *dns.Msg, now time.Time, d time.Duration) *item {
	i := &item{refreshState: new(refreshState)}
	if len(m.Question) != 0 {
		i.Name = m.Question[0].Name
		i.QType = m.Question[0].Qtype
		i.QClass = m.Question[0].Qclass
	}
	i.Rcode = m.Rcode
	i.Authoritative = m.Authoritative
	i.AuthenticatedData = m.AuthenticatedData
	i.RecursionAvailable = m.RecursionAvailable
	i.Answer = m.Answer
	i.Ns = m.Ns
	i.Extra = make([]dns.RR, len(m.Extra))
	// Don't copy OPT records as these are hop-by-hop.
	j := 0
	for _, e := range m.Extra {
		if e.Header().Rrtype == dns.TypeOPT {
			continue
		}
		i.Extra[j] = e
		j++
	}
	i.Extra = i.Extra[:j]
	i.answering = answersQuestion(m)

	i.origTTL = uint32(d.Seconds())
	// Keep the monotonic clock reading so TTL expiry is unaffected by wall
	// clock adjustments.
	i.stored = now

	i.Freq = new(freq.Freq)

	return i
}

// withLastKnownGood publishes a new holder without modifying readers of i.
// The acquisition is unchanged: share its payload, frequency and refresh
// lifecycle without copying used atomics or retaining a chain of old holders.
func (i *item) withLastKnownGood(answer *item) *item {
	return &item{
		Name: i.Name, QType: i.QType, QClass: i.QClass,
		Rcode: i.Rcode, Authoritative: i.Authoritative,
		AuthenticatedData: i.AuthenticatedData, RecursionAvailable: i.RecursionAvailable,
		Answer: i.Answer, Ns: i.Ns, Extra: i.Extra,
		wildcard: i.wildcard, do: i.do, answering: i.answering,
		lastKnownGood: answer, origTTL: i.origTTL, stored: i.stored,
		Freq: i.Freq, refreshState: i.refreshState,
	}
}

// toMsg turns i into a message, it tailors the reply to m.
func (i *item) toMsg(m *dns.Msg, now time.Time, do bool, ad bool) *dns.Msg {
	ttl := uint32(i.ttl(now)) // #nosec G115 -- ttl is bounded by DNS TTL limits
	return i.toMsgWithTTL(m, ttl, do, ad)
}

// toMsgWithTTL returns the cached item with an explicit TTL on every RR.
func (i *item) toMsgWithTTL(m *dns.Msg, ttl uint32, do bool, ad bool) *dns.Msg {
	m1 := new(dns.Msg)
	m1.SetReply(m)

	// The AA bit comes from the cached answer instead of being synthesized: a
	// reply served from cache is no more authoritative than the reply that
	// populated it (RFC 1035 section 4.1.1). It was hardcoded to 1 for legacy
	// stub resolvers that dropped non-authoritative answers, but that only ever
	// applied on the cache hit path, so those clients still saw AA=0 on every
	// miss. See #6185.
	m1.Authoritative = i.Authoritative
	m1.AuthenticatedData = i.AuthenticatedData
	if !do && !ad {
		// When DNSSEC was not wanted, it can't be authenticated data.
		// However, retain the AD bit if the requester set the AD bit, per RFC6840 5.7-5.8
		m1.AuthenticatedData = false
	}
	m1.RecursionAvailable = i.RecursionAvailable
	m1.Rcode = i.Rcode

	m1.Answer = filterRRSlice(i.Answer, ttl, true)
	m1.Ns = filterRRSlice(i.Ns, ttl, true)
	m1.Extra = filterRRSlice(i.Extra, ttl, true)
	if !do {
		m1.Answer = filterDNSSEC(m1.Answer, i.QType)
		m1.Ns = filterDNSSEC(m1.Ns, 0)
		m1.Extra = filterDNSSEC(m1.Extra, 0)
	}

	return m1
}

func (i *item) ttl(now time.Time) int {
	ttl := int(i.origTTL) - int(now.Sub(i.stored).Seconds())
	return ttl
}

func (i *item) matches(state request.Request) bool {
	if !i.do && requestDO(state.Req) {
		return false
	}
	return i.matchesQuestion(state)
}

func (i *item) matchesQuestion(state request.Request) bool {
	return state.QType() == i.QType && state.QClass() == i.QClass && strings.EqualFold(state.QName(), i.Name)
}

func (i *item) answersQuestion(state request.Request) bool {
	return i.answering && i.matches(state)
}

func (i *item) answeringItem(state request.Request) *item {
	if i.answersQuestion(state) {
		return i
	}
	if i.lastKnownGood != nil && i.lastKnownGood.answersQuestion(state) {
		return i.lastKnownGood
	}
	return nil
}

func (i *item) beginRefresh(now time.Time, failureRecheck time.Duration) bool {
	if failureRecheck > 0 {
		if retryAfter := i.retryAfter.Load(); retryAfter != nil && now.Before(*retryAfter) {
			return false
		}
	}
	return i.refreshing.CompareAndSwap(false, true)
}

func (i *item) endRefresh(now time.Time, failureRecheck time.Duration, refreshed bool) {
	if failureRecheck > 0 && !refreshed {
		retryAfter := now.Add(failureRecheck)
		i.retryAfter.Store(&retryAfter)
	} else {
		i.retryAfter.Store(nil)
	}
	// Publish the retry deadline before allowing another refresh to start.
	i.refreshing.Store(false)
}
