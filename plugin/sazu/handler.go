package sazu

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

// Sazu is the CoreDNS plugin implementing SAZU (Self-Authenticated Zone
// Update): a customer's own signer pushes DNSSEC-signed zone content,
// authenticated purely by SIG(0) (RFC 2931) riding on an RFC 2136 dynamic
// UPDATE, with no separate account/API-key handshake (§10.1/§10.2). See
// sazu-protocol.md for the full design; this implements enough of it --
// first-contact chain-of-trust bootstrap, full and partial pushes,
// in-memory serving -- to exercise the whole chain end to end. It does
// not implement the operational surface the design doc treats as
// separate concerns: rate limiting/quotas (§12), the §11 watch loop, key
// rollover (§10.4), or the HTTPS carrier (§7.3).
type Sazu struct {
	Next plugin.Handler

	Zones     []string
	Store     *Store
	Keys      *KeyRegistry
	Validator *Validator
	Capture   *RawCapture

	// InsecureSkipChainValidation disables the §10.2 chain-of-trust
	// cross-check at first contact. It exists purely for local testing,
	// where there is no real parent zone to publish a DS record against
	// -- see the onboarding guide. Never set true in production: with it
	// set, any self-signed key claiming any zone name is accepted on
	// first contact, which is exactly the spoofable behavior the
	// cross-check exists to prevent.
	InsecureSkipChainValidation bool

	// updateMu serializes the whole authenticate-evaluate-apply sequence
	// for UPDATE requests across all zones this instance serves. Simple
	// and correct for the request volumes this plugin is built to
	// exercise (onboarding and manual test pushes); a production version
	// would want a per-zone lock instead, for throughput, not correctness.
	updateMu sync.Mutex
}

func (s *Sazu) Name() string { return "sazu" }

// ServeDNS implements plugin.Handler.
func (s *Sazu) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if len(r.Question) != 1 {
		return plugin.NextOrFailure(s.Name(), s.Next, ctx, w, r)
	}
	zone := plugin.Zones(s.Zones).Matches(r.Question[0].Name)
	if zone == "" {
		return plugin.NextOrFailure(s.Name(), s.Next, ctx, w, r)
	}
	if r.Opcode == dns.OpcodeUpdate {
		return s.serveUpdate(w, r, zone)
	}
	return s.serveQuery(w, r, zone)
}

func (s *Sazu) serveQuery(w dns.ResponseWriter, r *dns.Msg, zone string) (int, error) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	z, ok := s.Store.Get(zone)
	if !ok || z.SOA() == nil {
		m.Rcode = dns.RcodeNameError
		return writeMsg(w, m)
	}

	q := r.Question[0]
	rrs := z.Lookup(q.Name, q.Qtype)
	if len(rrs) == 0 && !z.NameExists(q.Name) {
		m.Rcode = dns.RcodeNameError
	} else {
		m.Answer = rrs // NOERROR/NODATA when the name exists but this type doesn't
	}
	return writeMsg(w, m)
}

func (s *Sazu) serveUpdate(w dns.ResponseWriter, r *dns.Msg, zone string) (int, error) {
	reply := func(rcode int) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Rcode = rcode
		return writeMsg(w, m)
	}

	raw, ok := s.Capture.Take(w.RemoteAddr(), r.Id)
	if !ok {
		// No exact wire bytes captured for this request -- there is
		// nothing to verify a SIG(0) signature against. Fail closed
		// rather than trust a re-encoding of the parsed message.
		return reply(dns.RcodeServerFailure)
	}

	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	pinned, alreadyPinned := s.Keys.Get(zone)
	var candidate *dns.DNSKEY
	if alreadyPinned {
		candidate = pinned
	} else {
		var err error
		candidate, err = findCandidateKey(r.Ns, zone)
		if err != nil {
			return reply(dns.RcodeRefused)
		}
	}

	if err := VerifySIG0(raw, candidate); err != nil {
		return reply(dns.RcodeNotAuth)
	}

	if !alreadyPinned {
		if !s.InsecureSkipChainValidation {
			if err := s.Validator.VerifyChainOfTrust(zone, candidate); err != nil {
				return reply(dns.RcodeRefused)
			}
		}
		if !containsAPEXSOA(r.Ns, zone) {
			// A first-contact push that doesn't establish a real SOA
			// would pin a key for a zone with nothing servable behind
			// it. Reject before pinning anything.
			return reply(dns.RcodeFormatError)
		}
	}

	z := s.Store.GetOrCreate(zone)
	if rcode, err := EvaluatePrerequisites(z, r.Answer, dns.ClassINET); err != nil {
		_ = err // surfaced only via rcode; see EvaluatePrerequisites' doc comment
		return reply(rcode)
	}
	if err := ApplyUpdateOps(z, r.Ns, dns.ClassINET); err != nil {
		return reply(dns.RcodeFormatError)
	}

	if !alreadyPinned {
		s.Keys.Pin(zone, candidate)
	}
	return reply(dns.RcodeSuccess)
}

// findCandidateKey looks for exactly one Add-shaped DNSKEY at zone's apex
// among update ops -- the candidate key a first-contact push introduces
// itself with (§9.1: the same key signs and authenticates, so it always
// travels with the push).
func findCandidateKey(updateOps []dns.RR, zone string) (*dns.DNSKEY, error) {
	zoneLower := strings.ToLower(dns.Fqdn(zone))
	var found *dns.DNSKEY
	for _, rr := range updateOps {
		key, ok := rr.(*dns.DNSKEY)
		if !ok {
			continue
		}
		h := key.Header()
		if h.Rdlength == 0 || !strings.EqualFold(h.Name, zoneLower) {
			continue // a delete-shaped DNSKEY op, or for a different name
		}
		if found != nil {
			return nil, fmt.Errorf("more than one candidate DNSKEY in update")
		}
		found = key
	}
	if found == nil {
		return nil, fmt.Errorf("no candidate DNSKEY found for %s", zone)
	}
	return found, nil
}

// containsAPEXSOA reports whether updateOps adds a real SOA record at
// zone's apex.
func containsAPEXSOA(updateOps []dns.RR, zone string) bool {
	zoneLower := strings.ToLower(dns.Fqdn(zone))
	for _, rr := range updateOps {
		soa, ok := rr.(*dns.SOA)
		if !ok {
			continue
		}
		h := soa.Header()
		if h.Rdlength > 0 && strings.EqualFold(h.Name, zoneLower) {
			return true
		}
	}
	return false
}

func writeMsg(w dns.ResponseWriter, m *dns.Msg) (int, error) {
	if err := w.WriteMsg(m); err != nil {
		return dns.RcodeServerFailure, err
	}
	return dns.RcodeSuccess, nil
}
