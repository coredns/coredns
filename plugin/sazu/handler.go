package sazu

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
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
// UPDATE, with no separate account/API-key handshake (§10.1/§10.2), over
// UDP, TCP, or HTTPS (§7.3). See sazu-protocol.md for the full design;
// see SAZU-PLAN.md for exactly what of it this port implements today.
type Sazu struct {
	Next plugin.Handler

	Zones       []string
	Store       *Store
	Keys        *KeyRegistry
	Contacts    *ContactRegistry
	Validator   ChainValidator
	Capture     *RawCapture
	RateLimiter *RateLimiter

	// IPRateLimiter enforces a global, per-source-IP flood/scan throttle
	// (§12's ERR_RATE_LIMITED), independent of RateLimiter's per-zone
	// daily quota: it bounds total UPDATE attempt volume from one address
	// regardless of which zone name(s) it targets, closing the gap a
	// per-zone-only quota leaves open against an attacker probing many
	// different candidate zone names from one address. Checked before
	// anything else in serveUpdate -- before SIG(0) verification, even --
	// since it exists to bound raw attempt volume, not just successfully
	// authenticated ones.
	IPRateLimiter *IPRateLimiter

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
		return s.serveUpdate(ctx, w, r, qname)
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
	log.Debugf("query %s/%s (zone %s, DO=%v): %d matching RRset(s), nameExists=%v",
		q.Name, dns.TypeToString[q.Qtype], z.Origin, isDNSSECRequested(r), len(rrs), nameExists)
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
	log.Debugf("query %s/%s: replying rcode=%s answer=%v authority=%v",
		q.Name, dns.TypeToString[q.Qtype], dns.RcodeToString[m.Rcode], m.Answer, m.Ns)
	return writeMsg(w, m)
}

// isDNSSECRequested reports whether r carries the EDNS0 DO bit -- the
// signal a validating resolver (or any DNSSEC-aware client) sets to ask
// for RRSIGs alongside ordinary answers.
func isDNSSECRequested(r *dns.Msg) bool {
	opt := r.IsEdns0()
	return opt != nil && opt.Do()
}

func (s *Sazu) serveUpdate(ctx context.Context, w dns.ResponseWriter, r *dns.Msg, zone string) (int, error) {
	txID := newTransactionID()
	remoteAddr := w.RemoteAddr().String()
	// reply is the sole exit point for this function: every response,
	// accepted or refused, goes through it, so the §12 audit trail (when
	// s.DB is configured) sees every transaction this server decided on,
	// not just the successful ones -- an operator investigating "why did
	// my push fail" needs the rejected attempts at least as much as the
	// accepted ones. A logging failure here is deliberately never the
	// reason an UPDATE itself fails: it's just logged, since the audit
	// trail is a record of what happened, not a gate on whether it can.
	reply := func(rcode int, status string) (int, error) {
		if s.DB != nil {
			entry := AuditEntry{ID: txID, Zone: zone, RemoteAddr: remoteAddr, Rcode: dns.RcodeToString[rcode], Status: status, At: time.Now()}
			if err := s.DB.RecordTransaction(entry); err != nil {
				log.Errorf("update for %s: recording audit entry %s: %v", zone, txID, err)
			}
		}
		return replyWithStatus(w, r, rcode, status)
	}

	log.Debugf("update for %s from %s: transaction %s, %d prerequisite(s), %d op(s)", zone, remoteAddr, txID, len(r.Answer), len(r.Ns))

	if s.IPRateLimiter != nil && !s.IPRateLimiter.Allow(remoteAddr) {
		// Checked before anything else -- SIG(0) verification included --
		// deliberately: this bounds raw attempt volume from remoteAddr
		// regardless of whether the attempt is even well-formed, which is
		// exactly what a flood/scan guard needs. See IPRateLimiter's own
		// doc comment for the gap this closes that RateLimiter's per-zone
		// quota (checked later, and only after SIG(0) verifies) cannot:
		// an attacker varying the target zone name gets a fresh quota
		// bucket every time, but never a fresh IPRateLimiter bucket.
		log.Warningf("update for %s from %s: rejected, source IP exceeded its update rate limit", zone, remoteAddr)
		return reply(dns.RcodeRefused, statusErrRateLimited)
	}

	raw, ok := s.Capture.Take(w.RemoteAddr(), r.Id)
	if !ok {
		// §7.3 HTTPS/JSON carrier: UDP/TCP get here via
		// UDPDecorateReaderFunc/TCPDecorateReaderFunc into s.Capture, but
		// HTTPS/HTTP3 never go through a dns.Server's DecorateReader at
		// all -- core/dnsserver's ServerHTTPS/ServerHTTPS3 instead stash
		// the exact wire bytes doh.RequestToMsgWireWithAccept already
		// extracted (already decoded out of a JSON wire envelope, if the
		// client used one) directly on the request context, precisely so
		// a plugin like this one -- whose SIG(0)/RFC 2931 authentication
		// must verify against literal wire bytes, never a re-encoding --
		// has something to check over that transport too.
		if httpRaw, isHTTP := ctx.Value(dnsserver.RawRequestKey{}).([]byte); isHTTP {
			raw, ok = httpRaw, true
		}
	}
	if !ok {
		// No exact wire bytes captured for this request -- there is
		// nothing to verify a SIG(0) signature against. Fail closed
		// rather than trust a re-encoding of the parsed message.
		log.Warningf("update for %s from %s: no raw bytes captured for id %d, refusing", zone, w.RemoteAddr(), r.Id)
		return reply(dns.RcodeServerFailure, "")
	}

	log.Debugf("update for %s: waiting for updateMu (serializes all zones on this instance)", zone)
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	log.Debugf("update for %s: acquired updateMu", zone)

	pinned, alreadyPinned := s.Keys.Get(zone)
	var candidate *dns.DNSKEY
	if alreadyPinned {
		candidate = pinned
	} else {
		var ferr error
		candidate, ferr = findCandidateKey(r.Ns, zone)
		if ferr != nil {
			log.Debugf("update for %s: no candidate DNSKEY found in a first-contact push: %v", zone, ferr)
			return reply(dns.RcodeRefused, "")
		}
		log.Debugf("update for %s: first-contact candidate key tag %d algorithm %d", zone, candidate.KeyTag(), candidate.Algorithm)
		if !algorithmMeetsFloor(candidate.Algorithm) {
			log.Debugf("update for %s: candidate key algorithm %d is below the minimum floor (RFC 8624 §3.1), refusing", zone, candidate.Algorithm)
			return reply(dns.RcodeRefused, statusErrWeakAlgorithm)
		}
	}

	isRollover := false
	sigErr := VerifySIG0(raw, candidate)
	if sigErr != nil && alreadyPinned {
		// §10.4 key rollover: the pinned key didn't authenticate this
		// transaction -- before giving up, check whether a *different*
		// candidate DNSKEY also present in these ops does. If so, this
		// zone already has a pinned key presenting a new one it can prove
		// current possession of; the caller still has to run the exact
		// same chain-of-trust recheck first contact requires (a matching
		// DS at the parent) before this actually takes effect. Reusing
		// first contact's whole trust model rather than also requiring
		// the *old* key's signature is deliberate: whoever can get a DS
		// published at the registrar already fully controls the
		// delegation regardless (the root of trust first contact itself
		// already rests on), so requiring only that same proof here
		// doesn't introduce a new attack surface beyond what first
		// contact already accepts. An ordinary push's failure mode is
		// unchanged: if there's no distinct, self-verifying candidate,
		// sigErr stays exactly what VerifySIG0 against the pinned key
		// returned.
		if other, ferr := findCandidateKey(r.Ns, zone); ferr == nil &&
			!(other.PublicKey == pinned.PublicKey && other.Algorithm == pinned.Algorithm) {
			log.Debugf("update for %s: rollover candidate key tag %d algorithm %d", zone, other.KeyTag(), other.Algorithm)
			if !algorithmMeetsFloor(other.Algorithm) {
				// Checked before spending any effort verifying its
				// signature or (further down) walking the chain of
				// trust -- same "cheap check first" reasoning as first
				// contact's own floor check above.
				log.Debugf("update for %s: rollover candidate key algorithm %d is below the minimum floor (RFC 8624 §3.1), refusing", zone, other.Algorithm)
				return reply(dns.RcodeRefused, statusErrWeakAlgorithm)
			}
			if verr := VerifySIG0(raw, other); verr == nil {
				candidate, isRollover, sigErr = other, true, nil
			}
		}
	}
	if sigErr != nil {
		log.Debugf("update for %s: SIG(0) verification failed: %v", zone, sigErr)
		return reply(dns.RcodeNotAuth, "")
	}
	log.Debugf("update for %s: SIG(0) verified (rollover=%v)", zone, isRollover)

	// §10.6 registration record: a contact address (if this push carries
	// one) rides the same authenticated UPDATE as everything else, at a
	// reserved owner name -- see contact.go. Stripped out here, before
	// anything below treats r.Ns as zone content: it needs SIG(0)'s
	// authentication (already checked above) but none of DNSSEC's, since
	// it is never served.
	zoneOps, contactUpdate, err := splitContactOps(r.Ns, zone)
	if err != nil {
		log.Debugf("update for %s: invalid contact directive: %v", zone, err)
		return reply(dns.RcodeFormatError, "")
	}

	if s.RateLimiter != nil {
		// §12 quota: a full-zone push (one carrying a DNSKEY at the apex --
		// true of every first-contact push, and of every full re-push,
		// since BuildFullZonePush always re-asserts it) and an ordinary
		// differential push-update are metered separately, since they cost
		// very different amounts of server effort. Checked here, before
		// the expensive first-contact chain-of-trust walk below, so an
		// already-exhausted quota doesn't also pay for that network round
		// trip.
		isFull := !alreadyPinned || containsAPEXDNSKEY(zoneOps, zone)
		if !s.RateLimiter.Allow(zone, isFull) {
			log.Debugf("update for %s: rejected, quota exceeded (full=%v)", zone, isFull)
			return reply(dns.RcodeRefused, statusErrQuotaExceeded)
		}
	}

	if !alreadyPinned || isRollover {
		// A first-contact or rollover attempt is the one operation in
		// this package expensive enough to be worth protecting against a
		// spoofed-source-address flood specifically: it triggers a real
		// outbound network walk (VerifyChainOfTrust, below). IPRateLimiter
		// already bounds attempt volume per apparent source address, but
		// that protection is only meaningful over a transport where an
		// attacker can't just forge a fresh source address on every
		// packet with zero proof of controlling it -- true of plain UDP,
		// not of TCP or HTTPS/HTTP3 (both TLS-over-TCP and QUIC require
		// their own handshake-based address validation before any real
		// work happens). Without this, an attacker could spoof a
		// different source address on every UDP packet, each one still
		// getting IPRateLimiter's full per-address budget and each still
		// costing this server a real outbound query to the DNS root/TLD
		// infrastructure -- turning a rate limiter meant to bound that
		// exact cost into no protection at all.
		if !connectionOriented(ctx, w) {
			log.Debugf("update for %s: refusing a %s attempt over a connectionless transport (spoofable source address) from %s",
				zone, candidateKindLabel(isRollover), remoteAddr)
			return reply(dns.RcodeRefused, statusErrTransportNotAllowed)
		}
		if !s.InsecureSkipChainValidation {
			log.Infof("update for %s: %s, starting chain-of-trust walk to the DNS root (this makes real outbound DNS queries and can take a while on a restricted network)", zone, candidateKindLabel(isRollover))
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
				return reply(dns.RcodeRefused, status)
			}
		}
		if !alreadyPinned && !containsAPEXSOA(zoneOps, zone) {
			// A first-contact push that doesn't establish a real SOA
			// would pin a key for a zone with nothing servable behind
			// it. Reject before pinning anything. Not required for a
			// rollover: the zone already has real content and a real SOA
			// from before, and a rollover push may legitimately carry
			// nothing but the new key itself.
			return reply(dns.RcodeFormatError, "")
		}
	}

	z := s.Store.GetOrCreate(zone)
	if rcode, status, err := EvaluatePrerequisites(z, r.Answer, dns.ClassINET); err != nil {
		log.Debugf("update for %s: prerequisite failed: %v", zone, err)
		return reply(rcode, status)
	}

	if s.RequireValidRRSIGs {
		// §4's "Level 2 -- full verification": confirm the content being
		// pushed is itself validly DNSSEC-signed, not just that the
		// transaction carrying it was. Off by default -- SIG(0) alone
		// ("Level 0 -- trust the pipe") is still a supported, simpler
		// mode -- but this is what actually determines whether a zone
		// will validate for real DNSSEC resolvers once served.
		if status, err := VerifySignedRRsets(candidate, zoneOps, dns.ClassINET, time.Now()); err != nil {
			log.Debugf("update for %s: RequireValidRRSIGs check failed: %v", zone, err)
			if status == "" {
				status = statusErrSigInvalid
			}
			return reply(dns.RcodeNotAuth, status)
		}
	}

	// Persist before mutating memory: if the disk write fails, memory
	// stays exactly as it was before this request, rather than the two
	// disagreeing about whether the update actually happened.
	if s.DB != nil {
		var keyToPin *dns.DNSKEY
		if !alreadyPinned || isRollover {
			keyToPin = candidate
		}
		if err := s.DB.CommitUpdate(zone, keyToPin, zoneOps, dns.ClassINET, contactUpdate); err != nil {
			log.Errorf("update for %s: DB.CommitUpdate failed: %v", zone, err)
			return reply(dns.RcodeServerFailure, "")
		}
		log.Debugf("update for %s: committed to DB", zone)
	}

	// Invalidate any existing NSEC chain before applying this update's own
	// ops -- see ZoneData.PurgeNSEC's doc comment for why. A full push's
	// own freshly signed NSEC records are among the ops ApplyUpdateOps is
	// about to insert, so they repopulate the chain immediately after.
	z.PurgeNSEC()
	if err := ApplyUpdateOps(z, zoneOps, dns.ClassINET); err != nil {
		log.Errorf("update for %s: ApplyUpdateOps failed: %v", zone, err)
		return reply(dns.RcodeFormatError, "")
	}

	if !alreadyPinned {
		s.Keys.Pin(zone, candidate)
		log.Infof("update for %s: onboarded and pinned to key tag %d", zone, candidate.KeyTag())
	} else if isRollover {
		s.Keys.Pin(zone, candidate)
		log.Infof("update for %s: rolled over, now pinned to key tag %d (was %d)", zone, candidate.KeyTag(), pinned.KeyTag())
	}
	if contactUpdate != nil && s.Contacts != nil {
		s.Contacts.Set(zone, contactUpdate.Addresses)
		log.Debugf("update for %s: contact registration updated (%d address(es))", zone, len(contactUpdate.Addresses))
	}
	log.Debugf("update for %s: accepted", zone)
	return reply(dns.RcodeSuccess, "")
}

// candidateKindLabel names what kind of candidate-key event this is, for
// log messages shared between first contact and rollover.
func candidateKindLabel(isRollover bool) string {
	if isRollover {
		return "key rollover"
	}
	return "first-contact"
}

// connectionOriented reports whether this UPDATE arrived over a
// transport that requires a completed handshake -- proof of actually
// controlling the claimed source address -- before either side can
// exchange any real data: TCP, or HTTPS/HTTP3 (both TLS-over-TCP and
// QUIC perform their own handshake-based address validation), as opposed
// to plain UDP, where a single forged packet can claim any source
// address at all with nothing to disprove it.
//
// HTTPS/HTTP3 is detected via dnsserver.RawRequestKey's presence on ctx
// -- set unconditionally by both ServeHTTP methods -- rather than by
// inspecting w.RemoteAddr()'s concrete net.Addr type: ServerHTTPS3
// happens to construct its DoHWriter's RemoteAddr as a *net.UDPAddr
// (QUIC itself runs over UDP), which would otherwise look
// indistinguishable from plain, spoofable UDP by address type alone,
// even though QUIC's own handshake makes it just as address-validated
// as TCP.
func connectionOriented(ctx context.Context, w dns.ResponseWriter) bool {
	if _, isHTTP := ctx.Value(dnsserver.RawRequestKey{}).([]byte); isHTTP {
		return true
	}
	addr := w.RemoteAddr()
	return addr != nil && addr.Network() == "tcp"
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

// containsAPEXDNSKEY reports whether updateOps adds a DNSKEY at zone's
// apex -- §12's signal for classifying a push as "full-zone" for rate-
// limiting purposes: BuildFullZonePush always includes one (first contact
// or not), while an ordinary push-update never does.
func containsAPEXDNSKEY(updateOps []dns.RR, zone string) bool {
	zoneLower := strings.ToLower(dns.Fqdn(zone))
	for _, rr := range updateOps {
		key, ok := rr.(*dns.DNSKEY)
		if !ok {
			continue
		}
		h := key.Header()
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
// section." Every status code in §12's list is implemented at this
// point (see the other statusErr* constants below and in prereq.go/
// sign.go); see SAZU-PLAN.md for the full accounting.
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

// statusErrWeakAlgorithm is another of §12's status codes: a first-contact
// candidate key's algorithm doesn't meet §10.7's minimum floor (RFC 8624
// §3.1) -- see algorithm.go.
const statusErrWeakAlgorithm = "ERR_WEAK_ALGORITHM"

// statusErrQuotaExceeded is another of §12's status codes: this zone has
// already used up its full-zone or differential push quota for the
// current rolling 24h window -- see RateLimiter and containsAPEXDNSKEY.
// §12 also names a distinct ERR_RATE_LIMITED code (see
// statusErrRateLimited) -- that one is a separate, faster-timescale,
// per-source-IP flood throttle, not just a synonym for this.
const statusErrQuotaExceeded = "ERR_QUOTA_EXCEEDED"

// statusErrRateLimited is §12's remaining status code: remoteAddr has
// exceeded IPRateLimiter's global, per-source-IP UPDATE rate over the
// current rolling 1-minute window -- distinct from statusErrQuotaExceeded
// (a per-*zone* daily churn quota, checked only after SIG(0) verifies)
// specifically because this one bounds raw attempt volume from an
// address regardless of which zone it targets or whether the attempt is
// even well-formed.
const statusErrRateLimited = "ERR_RATE_LIMITED"

// statusErrTransportNotAllowed: this first-contact or key-rollover
// attempt arrived over a connectionless transport (plain UDP) -- see
// connectionOriented. Not one of §12's named codes (the design doc
// predates the HTTPS/JSON carrier and this specific spoofing concern),
// but the same diagnostic-TXT convention as the rest of them.
const statusErrTransportNotAllowed = "ERR_TRANSPORT_NOT_ALLOWED"

// statusErrStaleSerial is another of §12's status codes: a push built
// with BuildFullZonePush's previousSOA staleness guard (RFC 2136 §2.4.2)
// was rejected because the zone's current SOA no longer matches what the
// push was built against -- see EvaluatePrerequisites.
const statusErrStaleSerial = "ERR_STALE_SERIAL"

// statusErrExpiredSignature is another of §12's status codes: distinct
// from the more general statusErrSigInvalid -- emitted only when
// RequireValidRRSIGs is enabled and a pushed RRset's RRSIG would
// otherwise verify against the candidate/pinned key (right name, type,
// key tag, and algorithm, and a cryptographically valid signature) but
// falls outside its own inception/expiration window. Telling this apart
// from "no valid signature at all" matters operationally: this one means
// "re-sign and re-push," not "something is wrong with the key or the
// content."
const statusErrExpiredSignature = "ERR_EXPIRED_SIGNATURE"

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
