package sazu

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/miekg/dns"
)

// log follows the same convention as every other CoreDNS plugin (see
// e.g. plugin/hosts): log.Info/Warning/Error are always visible; log.Debug
// only prints once the Corefile also loads the `debug` plugin. There was
// no logging anywhere in this plugin before -- added specifically because
// a real production hang (a first-contact push that got no response at
// all, even after a minute) turned out to be undiagnosable without it:
// nothing here distinguished "stuck in the chain-of-trust network walk"
// from "silently dropped" from the outside.
var log = clog.NewWithPlugin("sazu")

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
	Validator ChainValidator
	Capture   *RawCapture

	// DB, if non-nil, persists every accepted UPDATE (see db.go): a
	// restart replays it back into Store/Keys instead of starting empty.
	// Nil is a fully supported mode -- purely in-memory, matching every
	// behavior this plugin had before persistence existed (what all of
	// this package's unit tests still use).
	DB *DB

	// InsecureSkipChainValidation disables the §10.2 chain-of-trust
	// cross-check at first contact. It exists purely for local testing,
	// where there is no real parent zone to publish a DS record against
	// -- see the onboarding guide. Never set true in production: with it
	// set, any self-signed key claiming any zone name is accepted on
	// first contact, which is exactly the spoofable behavior the
	// cross-check exists to prevent.
	InsecureSkipChainValidation bool

	// RequireValidRRSIGs enables §4's "Level 2 -- full verification":
	// every Add-shaped RRset in an UPDATE must carry a covering RRSIG
	// that actually verifies against the candidate/pinned key, or the
	// whole update is rejected. Off by default -- "Level 0, trust the
	// pipe" (SIG(0) authenticates the push, nothing checks the content
	// itself is validly signed) is still a supported, simpler mode, and
	// is what every test in this package other than the ones specifically
	// about this flag exercises.
	RequireValidRRSIGs bool

	// updateMu serializes the whole authenticate-evaluate-apply sequence
	// for UPDATE requests across all zones this instance serves. Simple
	// and correct for the request volumes this plugin is built to
	// exercise (onboarding and manual test pushes); a production version
	// would want a per-zone lock instead, for throughput, not correctness.
	updateMu sync.Mutex
}

func (s *Sazu) Name() string { return "sazu" }

// ServeDNS implements plugin.Handler.
// ServeDNS implements plugin.Handler. s.Zones (from the Corefile) sets
// this instance's *static scope* -- e.g. "." to accept any domain at
// all, or a narrower umbrella zone to only accept subdomains delegated
// under one zone -- and is deliberately kept separate from which zones
// have actually been onboarded (dynamic, in s.Store): that separation is
// what lets a new customer domain be onboarded by sending it a signed
// push, with no Corefile edit or server restart needed per domain.
func (s *Sazu) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if len(r.Question) != 1 {
		return plugin.NextOrFailure(s.Name(), s.Next, ctx, w, r)
	}
	qname := r.Question[0].Name

	if r.Opcode == dns.OpcodeUpdate {
		// RFC 2136: the "question" of an UPDATE message is the zone
		// section, naming the zone directly -- no suffix matching, and
		// it may well be a zone never seen before (first contact).
		if plugin.Zones(s.Zones).Matches(qname) == "" {
			return plugin.NextOrFailure(s.Name(), s.Next, ctx, w, r)
		}
		return s.serveUpdate(w, r, qname)
	}

	// Ordinary query: find which *onboarded* zone (if any) qname falls
	// under -- a lookup against live state, not the static Corefile
	// list, since many customer zones can share one broad "sazu ." scope.
	// A qname within s.Zones' scope but never actually onboarded falls
	// through to Next rather than NXDOMAIN, so a broad scope like "."
	// doesn't swallow every other zone/plugin on the same server.
	_, z, ok := s.Store.FindZoneForName(qname)
	if !ok {
		return plugin.NextOrFailure(s.Name(), s.Next, ctx, w, r)
	}
	return s.serveQuery(w, r, z)
}

func (s *Sazu) serveQuery(w dns.ResponseWriter, r *dns.Msg, z *ZoneData) (int, error) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	q := r.Question[0]
	rrs := z.Lookup(q.Name, q.Qtype)
	nameExists := z.NameExists(q.Name)
	if len(rrs) == 0 && !nameExists {
		m.Rcode = dns.RcodeNameError
	} else {
		m.Answer = rrs // NOERROR/NODATA when the name exists but this type doesn't
		if len(rrs) > 0 && isDNSSECRequested(r) {
			// A validating resolver needs the covering RRSIG(s) in the
			// *same* answer as the RRset they cover, not as a separate
			// query -- without this, the zone would carry real
			// signatures (once actually pushed signed) that never
			// reached anyone asking for them, still producing exactly
			// the "RRSIGs Missing" bogus state this whole feature exists
			// to avoid.
			m.Answer = append(m.Answer, z.LookupRRSIG(q.Name, q.Qtype)...)
		}
	}

	if len(rrs) == 0 {
		// RFC 2308 §3: every negative response (NXDOMAIN here, or NODATA
		// in the "name exists but not this type" branch above) MUST
		// carry the zone's SOA in the authority section, so a resolver
		// knows how long it may cache the negative result for. Omitting
		// it doesn't make the *answer* wrong, but it silently defeats
		// negative caching -- every repeat query for the same
		// nonexistent name or type would otherwise bypass cache and hit
		// this server directly every time.
		if soa := z.SOA(); soa != nil {
			m.Ns = append(m.Ns, soa)
			if isDNSSECRequested(r) {
				m.Ns = append(m.Ns, z.LookupRRSIG(z.Origin, dns.TypeSOA)...)
				// RFC 4035 §3.1.3: the authenticated denial-of-existence
				// proof itself, without which a validating resolver has
				// to treat this negative answer as Bogus rather than
				// Insecure or Secure once a DS is published for this
				// zone. See ZoneData.NegativeProof and nsec.go's
				// top-of-file comment for why this can be empty (no NSEC
				// chain currently exists) even on a zone that has one on
				// other names, and why that's a safe degradation rather
				// than a bug.
				m.Ns = append(m.Ns, z.NegativeProof(q.Name, nameExists)...)
			}
		}
	}
	return writeMsg(w, m)
}

// isDNSSECRequested reports whether r carries the EDNS0 DO bit -- the
// signal a validating resolver (or any DNSSEC-aware client) sets to ask
// for RRSIGs alongside ordinary answers.
func isDNSSECRequested(r *dns.Msg) bool {
	opt := r.IsEdns0()
	return opt != nil && opt.Do()
}

func (s *Sazu) serveUpdate(w dns.ResponseWriter, r *dns.Msg, zone string) (int, error) {
	reply := func(rcode int) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Rcode = rcode
		return writeMsg(w, m)
	}

	log.Debugf("update for %s from %s: %d prerequisite(s), %d op(s)", zone, w.RemoteAddr(), len(r.Answer), len(r.Ns))

	raw, ok := s.Capture.Take(w.RemoteAddr(), r.Id)
	if !ok {
		// No exact wire bytes captured for this request -- there is
		// nothing to verify a SIG(0) signature against. Fail closed
		// rather than trust a re-encoding of the parsed message.
		log.Warningf("update for %s from %s: no raw bytes captured for id %d, refusing", zone, w.RemoteAddr(), r.Id)
		return reply(dns.RcodeServerFailure)
	}

	log.Debugf("update for %s: waiting for updateMu (serializes all zones on this instance)", zone)
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	log.Debugf("update for %s: acquired updateMu", zone)

	pinned, alreadyPinned := s.Keys.Get(zone)
	var candidate *dns.DNSKEY
	if alreadyPinned {
		candidate = pinned
		log.Debugf("update for %s: zone already pinned to key tag %d", zone, candidate.KeyTag())
	} else {
		var err error
		candidate, err = findCandidateKey(r.Ns, zone)
		if err != nil {
			log.Debugf("update for %s: no candidate DNSKEY found in a first-contact push: %v", zone, err)
			return reply(dns.RcodeRefused)
		}
		log.Debugf("update for %s: first-contact candidate key tag %d algorithm %d", zone, candidate.KeyTag(), candidate.Algorithm)
	}

	if err := VerifySIG0(raw, candidate); err != nil {
		log.Debugf("update for %s: SIG(0) verification failed: %v", zone, err)
		return reply(dns.RcodeNotAuth)
	}
	log.Debugf("update for %s: SIG(0) verified", zone)

	if !alreadyPinned {
		if !s.InsecureSkipChainValidation {
			log.Infof("update for %s: first contact, starting chain-of-trust walk to the DNS root (this makes real outbound DNS queries and can take a while on a restricted network)", zone)
			start := time.Now()
			err := s.Validator.VerifyChainOfTrust(zone, candidate)
			log.Infof("update for %s: chain-of-trust walk finished in %s, err=%v", zone, time.Since(start), err)
			if err != nil {
				status := ""
				if ce, ok := err.(*ChainError); ok {
					switch ce.Op {
					case "no-ds-published":
						// §12's status-code convention: the specific, by far
						// most common first-contact failure -- "you haven't
						// told your registrar about this key yet" -- gets its
						// own diagnostic so a client can say exactly that,
						// rather than a bare REFUSED indistinguishable from a
						// wrong key or a broken chain elsewhere.
						status = statusErrNoDSPublished
					case "key-mismatch":
						// A DS *is* published for this zone, just not for
						// this key. Distinct from ERR_NO_DS_PUBLISHED and
						// deliberately not phrased as "wrong key" or
						// "attack": the DS found here may well be
						// legitimate DNSSEC this zone's current host
						// already publishes under its own key, unrelated
						// to SAZU entirely -- exactly the case a client
						// needs flagged rather than silently lumped in
						// with a bare REFUSED.
						status = statusErrUnknownSigner
					}
				}
				return replyWithStatus(w, r, dns.RcodeRefused, status)
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
		log.Debugf("update for %s: prerequisite failed: %v", zone, err)
		return reply(rcode)
	}

	if s.RequireValidRRSIGs {
		// §4's "Level 2 -- full verification": confirm the content being
		// pushed is itself validly DNSSEC-signed, not just that the
		// transaction carrying it was. Off by default -- SIG(0) alone
		// ("Level 0 -- trust the pipe") is still a supported, simpler
		// mode -- but this is what actually determines whether a zone
		// will validate for real DNSSEC resolvers once served.
		if err := VerifySignedRRsets(candidate, r.Ns, dns.ClassINET, time.Now()); err != nil {
			log.Debugf("update for %s: RequireValidRRSIGs check failed: %v", zone, err)
			return replyWithStatus(w, r, dns.RcodeNotAuth, statusErrSigInvalid)
		}
	}

	// Persist before mutating memory: if the disk write fails, memory
	// stays exactly as it was before this request, rather than the two
	// disagreeing about whether the update actually happened.
	if s.DB != nil {
		var keyToPin *dns.DNSKEY
		if !alreadyPinned {
			keyToPin = candidate
		}
		if err := s.DB.CommitUpdate(zone, keyToPin, r.Ns, dns.ClassINET); err != nil {
			log.Errorf("update for %s: DB.CommitUpdate failed: %v", zone, err)
			return reply(dns.RcodeServerFailure)
		}
		log.Debugf("update for %s: committed to DB", zone)
	}

	// Invalidate any existing NSEC chain before applying this update's own
	// ops -- see ZoneData.PurgeNSEC's doc comment for why. A full push's
	// own freshly signed NSEC records are among the ops ApplyUpdateOps is
	// about to insert, so they repopulate the chain immediately after.
	z.PurgeNSEC()
	if err := ApplyUpdateOps(z, r.Ns, dns.ClassINET); err != nil {
		log.Errorf("update for %s: ApplyUpdateOps failed: %v", zone, err)
		return reply(dns.RcodeFormatError)
	}

	if !alreadyPinned {
		s.Keys.Pin(zone, candidate)
		log.Infof("update for %s: onboarded and pinned to key tag %d", zone, candidate.KeyTag())
	}
	log.Debugf("update for %s: accepted", zone)
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

// statusErrNoDSPublished is one of §12's SAZU status codes, carried as a
// diagnostic TXT record per that section: "On the raw-DNS carrier this
// rides as a short diagnostic TXT record in the response's Additional
// section." Only this one code is implemented today -- the rest of §12's
// list (ERR_STALE_SERIAL, ERR_UNKNOWN_SIGNER, etc.) is still outstanding,
// see SAZU-PLAN.md.
const statusErrNoDSPublished = "ERR_NO_DS_PUBLISHED"

// statusErrUnknownSigner is another of §12's status codes: a DS record is
// published for the target zone, but none of them match the candidate
// key. Unlike statusErrNoDSPublished, this does not mean "nothing is
// there yet" -- something else already has DNSSEC set up for this zone,
// which the operator needs to understand (it may simply be the zone's
// current host, e.g. mid-migration) before doing anything that might
// disturb it.
const statusErrUnknownSigner = "ERR_UNKNOWN_SIGNER"

// statusErrSigInvalid is another of §12's status codes: emitted only when
// RequireValidRRSIGs is enabled and a pushed RRset's RRSIG doesn't
// actually verify against the candidate/pinned key.
const statusErrSigInvalid = "ERR_SIG_INVALID"

// replyWithStatus replies to r with rcode and, if status is non-empty,
// a diagnostic TXT record carrying it in the Additional section.
func replyWithStatus(w dns.ResponseWriter, r *dns.Msg, rcode int, status string) (int, error) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Rcode = rcode
	if status != "" {
		m.Extra = append(m.Extra, &dns.TXT{
			Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0},
			Txt: []string{status},
		})
	}
	return writeMsg(w, m)
}
