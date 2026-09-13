package sazu

import (
	"context"
	"crypto/ed25519"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

// newTestSazu wires a Sazu instance the way setup.go would, with
// InsecureSkipChainValidation on -- this test exercises onboarding,
// full pushes, partial pushes, and serving, not the chain-of-trust walk
// itself (which has its own dedicated tests and needs a real network).
func newTestSazu(zone string) *Sazu {
	return &Sazu{
		Zones:                       []string{dns.Fqdn(zone)},
		Store:                       NewStore(),
		Keys:                        NewKeyRegistry(),
		Validator:                   NewValidator(),
		Capture:                     NewRawCapture(5*time.Second, 64),
		InsecureSkipChainValidation: true,
	}
}

// serveThroughRealServer starts a real dnsserver.Server (as CoreDNS
// itself would) with s installed as the sole plugin, so its
// UDPDecorateReaderFunc-based raw capture is genuinely exercised over a
// real UDP round trip -- not called directly, which would prove nothing
// about the wiring this depends on.
func serveThroughRealServer(t *testing.T, s *Sazu) string {
	t.Helper()
	cfg := &dnsserver.Config{
		Zone:        s.Zones[0],
		Transport:   "dns",
		ListenHosts: []string{"127.0.0.1"},
		Port:        "0",
	}
	cfg.AddPlugin(func(next plugin.Handler) plugin.Handler {
		s.Next = next
		return s
	})
	cfg.AllowOpcode(dns.OpcodeUpdate)
	cfg.UDPDecorateReaderFunc = s.Capture.DecorateReaderFunc
	cfg.UDPSize = maxUDPMessageSize // see setup.go: a signed push regularly exceeds the 512B default

	srv, err := dnsserver.NewServer("127.0.0.1:0", []*dnsserver.Config{cfg})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() { _ = srv.ServePacket(pc) }()
	t.Cleanup(func() { _ = srv.Stop() })
	return pc.LocalAddr().String()
}

// sendRaw sends wire directly over UDP to addr and returns the response,
// bypassing dns.Client/dns.Exchange -- which would re-pack the message
// and defeat the entire point of testing byte-exact SIG(0) delivery.
func sendRaw(t *testing.T, addr string, wire []byte) *dns.Msg {
	t.Helper()
	conn, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write(wire); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	resp := new(dns.Msg)
	if err := resp.Unpack(buf[:n]); err != nil {
		t.Fatalf("Unpack response: %v", err)
	}
	return resp
}

func query(t *testing.T, addr, name string, qtype uint16) *dns.Msg {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	resp, _, err := new(dns.Client).Exchange(m, addr)
	if err != nil {
		t.Fatalf("query %s/%d: %v", name, qtype, err)
	}
	return resp
}

func queryDO(t *testing.T, addr, name string, qtype uint16) *dns.Msg {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	m.SetEdns0(4096, true)
	resp, _, err := new(dns.Client).Exchange(m, addr)
	if err != nil {
		t.Fatalf("query(DO) %s/%d: %v", name, qtype, err)
	}
	return resp
}

// TestOnboardedZoneServesRRSIGsWithDOBit proves a full onboarding push's
// real signatures actually reach a validating client: once onboarded (via
// BuildFullZonePush, which signs everything), a DO-bit query for A gets
// back both the A record and its covering RRSIG in the same answer --
// what a real validating resolver needs, and specifically what was
// missing when this was tested against a real domain (a published DS
// with no RRSIGs served at all produces exactly the "bogus"/SERVFAIL
// state this closes). A query without the DO bit gets no RRSIG, matching
// ordinary non-DNSSEC client expectations.
func TestOnboardedZoneServesRRSIGsWithDOBit(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	soa := testSOA(1)
	rrs := []dns.RR{testA("www.example.org.", net.IPv4(203, 0, 113, 10))}
	push, err := BuildFullZonePush("example.org.", soa, rrs, key, priv, nil)
	if err != nil {
		t.Fatalf("building push: %v", err)
	}
	now := time.Now()
	wire, err := SignUpdate(push, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	if resp := sendRaw(t, addr, wire); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("onboarding push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	withDO := queryDO(t, addr, "www.example.org.", dns.TypeA)
	var aRRs []dns.RR
	var sig *dns.RRSIG
	for _, rr := range withDO.Answer {
		switch v := rr.(type) {
		case *dns.A:
			aRRs = append(aRRs, v)
		case *dns.RRSIG:
			if v.TypeCovered == dns.TypeA {
				sig = v
			}
		}
	}
	if len(aRRs) == 0 || sig == nil {
		t.Fatalf("expected both the A record and its covering RRSIG with DO set, got %+v", withDO.Answer)
	}
	if err := sig.Verify(key, aRRs); err != nil {
		t.Fatalf("the served RRSIG does not verify against the served A record: %v", err)
	}

	withoutDO := query(t, addr, "www.example.org.", dns.TypeA)
	for _, rr := range withoutDO.Answer {
		if _, ok := rr.(*dns.RRSIG); ok {
			t.Fatalf("expected no RRSIG without the DO bit, got %+v", withoutDO.Answer)
		}
	}
}

// buildUnsignedFirstContactPush builds a first-contact UPDATE that
// establishes candidate/SOA/content exactly like a real onboarding push,
// but with none of it carrying an RRSIG -- only the SIG(0) transaction
// signature over the whole message is genuine. It exists to distinguish
// "Level 0: trust the pipe" (SIG(0) alone) from "Level 2: full
// verification" (the content itself must be validly signed), which is
// exactly what RequireValidRRSIGs toggles between.
func buildUnsignedFirstContactPush(t *testing.T, zone string, key *dns.DNSKEY, priv ed25519.PrivateKey) []byte {
	t.Helper()
	m := new(dns.Msg)
	m.SetUpdate(dns.Fqdn(zone))
	soa := synthesizeSOA(zone)
	rrs := []dns.RR{testA("www."+dns.Fqdn(zone), net.IPv4(203, 0, 113, 10))}
	adds := append([]dns.RR{key, soa}, rrs...)
	m.Insert(adds)
	now := time.Now()
	wire, err := SignUpdate(m, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing unsigned-content push: %v", err)
	}
	return wire
}

// TestRequireValidRRSIGsRejectsUnsignedContent proves the opt-in Level 2
// check actually gates on it: a push whose transaction is genuinely
// SIG(0)-signed but whose content carries no RRSIGs at all must be
// rejected once RequireValidRRSIGs is on.
func TestRequireValidRRSIGsRejectsUnsignedContent(t *testing.T) {
	s := newTestSazu("example.org.")
	s.RequireValidRRSIGs = true
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	wire := buildUnsignedFirstContactPush(t, "example.org.", key, priv)

	resp := sendRaw(t, addr, wire)
	if resp.Rcode != dns.RcodeNotAuth {
		t.Fatalf("rcode = %s, want NotAuth", dns.RcodeToString[resp.Rcode])
	}
	if status, ok := diagnosticStatus(resp); !ok || status != statusErrSigInvalid {
		t.Fatalf("diagnostic status = %q, ok=%v, want %q", status, ok, statusErrSigInvalid)
	}
	if _, ok := s.Keys.Get("example.org."); ok {
		t.Fatalf("a rejected push must not pin a key")
	}
}

// TestRequireValidRRSIGsOffAcceptsUnsignedContent proves the flag is
// genuinely opt-in: with it left at its default (false), the exact same
// unsigned-content push that the previous test rejects is accepted --
// "Level 0, trust the pipe" is still a supported mode.
func TestRequireValidRRSIGsOffAcceptsUnsignedContent(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	wire := buildUnsignedFirstContactPush(t, "example.org.", key, priv)

	resp := sendRaw(t, addr, wire)
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if _, ok := s.Keys.Get("example.org."); !ok {
		t.Fatalf("expected the candidate key to be pinned after a successful first-contact push")
	}
}

// TestOnboardFullPushThenQuery is the whole-chain proof: a first-contact
// full-zone push is accepted (with chain-of-trust validation skipped, the
// one piece that needs a real network -- see chain_test.go for that in
// isolation), pins the client's key, and the pushed records become
// genuinely servable over a real UDP query.
func TestOnboardFullPushThenQuery(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	soa := testSOA(1)
	rrs := []dns.RR{
		&dns.NS{Hdr: dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 3600}, Ns: "ns1.example.org."},
		testA("www.example.org.", net.IPv4(203, 0, 113, 10)),
	}
	push, err := BuildFullZonePush("example.org.", soa, rrs, key, priv, nil)
	if err != nil {
		t.Fatalf("building push: %v", err)
	}
	now := time.Now()
	wire, err := SignUpdate(push, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	resp := sendRaw(t, addr, wire)
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("onboarding push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	if pinned, ok := s.Keys.Get("example.org."); !ok || pinned.PublicKey != key.PublicKey {
		t.Fatalf("expected the candidate key to be pinned after a successful first-contact push")
	}

	answer := query(t, addr, "www.example.org.", dns.TypeA)
	if len(answer.Answer) != 1 {
		t.Fatalf("expected exactly one answer for www.example.org./A, got %d", len(answer.Answer))
	}
	a, ok := answer.Answer[0].(*dns.A)
	if !ok || !a.A.Equal(net.IPv4(203, 0, 113, 10)) {
		t.Fatalf("unexpected answer: %+v", answer.Answer[0])
	}

	soaAnswer := query(t, addr, "example.org.", dns.TypeSOA)
	if len(soaAnswer.Answer) != 1 {
		t.Fatalf("expected the pushed SOA to be servable, got %d answers", len(soaAnswer.Answer))
	}
}

// TestOnboardWithoutSOAIsRejected proves the "first contact must
// establish a real SOA" guard: a push with a candidate DNSKEY but no SOA
// content must be refused, and must not pin a key for a zone with
// nothing behind it.
func TestOnboardWithoutSOAIsRejected(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeSOA)
	m.Opcode = dns.OpcodeUpdate
	m.Insert([]dns.RR{
		&dns.DNSKEY{Hdr: dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
			Flags: key.Flags, Protocol: key.Protocol, Algorithm: key.Algorithm, PublicKey: key.PublicKey},
		testA("www.example.org.", net.IPv4(203, 0, 113, 10)),
	})
	now := time.Now()
	wire, err := SignUpdate(m, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	resp := sendRaw(t, addr, wire)
	if resp.Rcode == dns.RcodeSuccess {
		t.Fatalf("expected a first-contact push with no SOA to be rejected")
	}
	if _, ok := s.Keys.Get("example.org."); ok {
		t.Fatalf("expected no key to be pinned for a rejected first-contact push")
	}
}

// TestOrdinaryPartialPushAfterOnboarding is the second half of the whole
// chain: once a zone is onboarded, an ordinary push signed by the same
// (already-pinned) key -- carrying no DNSKEY at all -- can add and
// remove individual records without re-verifying chain-of-trust.
func TestOrdinaryPartialPushAfterOnboarding(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	soa := testSOA(1)
	rrs := []dns.RR{testA("www.example.org.", net.IPv4(203, 0, 113, 10))}
	onboard, err := BuildFullZonePush("example.org.", soa, rrs, key, priv, nil)
	if err != nil {
		t.Fatalf("building onboarding push: %v", err)
	}
	now := time.Now()
	wire, err := SignUpdate(onboard, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing onboarding push: %v", err)
	}
	if resp := sendRaw(t, addr, wire); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("onboarding push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	// Now a partial push: add a second record, remove the first -- no
	// DNSKEY, signed with the same key the server already pinned.
	partial := new(dns.Msg)
	partial.SetQuestion("example.org.", dns.TypeSOA)
	partial.Opcode = dns.OpcodeUpdate
	partial.Insert([]dns.RR{testA("mail.example.org.", net.IPv4(203, 0, 113, 20))})
	partial.Remove([]dns.RR{testA("www.example.org.", net.IPv4(203, 0, 113, 10))})

	now = time.Now()
	partialWire, err := SignUpdate(partial, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing partial push: %v", err)
	}
	resp := sendRaw(t, addr, partialWire)
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("partial push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	mailAnswer := query(t, addr, "mail.example.org.", dns.TypeA)
	if len(mailAnswer.Answer) != 1 {
		t.Fatalf("expected the partially-added record to be servable, got %d answers", len(mailAnswer.Answer))
	}
	wwwAnswer := query(t, addr, "www.example.org.", dns.TypeA)
	if len(wwwAnswer.Answer) != 0 {
		t.Fatalf("expected the partially-removed record to be gone, got %d answers", len(wwwAnswer.Answer))
	}
}

// TestPartialPushFromWrongKeyRejected proves a partial push can't
// impersonate an already-onboarded zone with a different key.
func TestPartialPushFromWrongKeyRejected(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	onboard, err := BuildFullZonePush("example.org.", testSOA(1), nil, key, priv, nil)
	if err != nil {
		t.Fatalf("building onboarding push: %v", err)
	}
	now := time.Now()
	wire, err := SignUpdate(onboard, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing onboarding push: %v", err)
	}
	if resp := sendRaw(t, addr, wire); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("onboarding push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	otherKey, otherPriv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating other key: %v", err)
	}
	partial := new(dns.Msg)
	partial.SetQuestion("example.org.", dns.TypeSOA)
	partial.Opcode = dns.OpcodeUpdate
	partial.Insert([]dns.RR{testA("evil.example.org.", net.IPv4(198, 51, 100, 1))})

	now = time.Now()
	partialWire, err := SignUpdate(partial, otherKey, otherPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	resp := sendRaw(t, addr, partialWire)
	if resp.Rcode == dns.RcodeSuccess {
		t.Fatalf("expected a push signed by an unpinned key to be rejected")
	}
}

// fakeValidator lets tests control exactly what VerifyChainOfTrust returns,
// so the response-shaping logic in serveUpdate (in particular the
// ERR_NO_DS_PUBLISHED diagnostic) can be tested without real network
// queries -- chain.go's own tests already cover the real network-calling
// logic; this covers what handler.go does with its result.
type fakeValidator struct {
	err error
}

func (f fakeValidator) VerifyChainOfTrust(string, *dns.DNSKEY) error { return f.err }

// TestOnboardDeniedWithNoDSPublishedGivesDiagnostic proves the exact
// behavior the onboarding UX depends on: a first-contact push for a zone
// with no DS published yet is refused, and carries a machine-readable
// ERR_NO_DS_PUBLISHED diagnostic a client can act on -- distinct from any
// other reason a push might be refused.
func TestOnboardDeniedWithNoDSPublishedGivesDiagnostic(t *testing.T) {
	s := newTestSazu("example.org.")
	s.InsecureSkipChainValidation = false
	s.Validator = fakeValidator{err: &ChainError{Op: "no-ds-published", Msg: "no DS record published yet for example.org."}}
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	push, err := BuildFullZonePush("example.org.", testSOA(1), nil, key, priv, nil)
	if err != nil {
		t.Fatalf("building push: %v", err)
	}
	now := time.Now()
	wire, err := SignUpdate(push, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	resp := sendRaw(t, addr, wire)
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("rcode = %s, want REFUSED", dns.RcodeToString[resp.Rcode])
	}
	status, ok := diagnosticStatus(resp)
	if !ok || status != statusErrNoDSPublished {
		t.Fatalf("expected an %s diagnostic TXT record, got status=%q ok=%v (extra=%+v)",
			statusErrNoDSPublished, status, ok, resp.Extra)
	}
	if _, pinned := s.Keys.Get("example.org."); pinned {
		t.Fatalf("expected no key to be pinned for a denied first-contact push")
	}
}

// TestOnboardDeniedWithKeyMismatchGivesDiagnostic proves the second
// dedicated diagnostic: a DS *is* published for the zone, just not one
// matching the candidate key -- distinct from ERR_NO_DS_PUBLISHED, since
// the fix isn't "go publish a DS" (one already exists) but "find out what
// already has DNSSEC set up for this zone" (possibly the zone's current
// host, mid-migration, rather than an attacker).
func TestOnboardDeniedWithKeyMismatchGivesDiagnostic(t *testing.T) {
	s := newTestSazu("example.org.")
	s.InsecureSkipChainValidation = false
	s.Validator = fakeValidator{err: &ChainError{Op: "key-mismatch", Msg: "a DS record is published for example.org., but none of them match the candidate key"}}
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	push, err := BuildFullZonePush("example.org.", testSOA(1), nil, key, priv, nil)
	if err != nil {
		t.Fatalf("building push: %v", err)
	}
	now := time.Now()
	wire, err := SignUpdate(push, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	resp := sendRaw(t, addr, wire)
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("rcode = %s, want REFUSED", dns.RcodeToString[resp.Rcode])
	}
	status, ok := diagnosticStatus(resp)
	if !ok || status != statusErrUnknownSigner {
		t.Fatalf("expected an %s diagnostic TXT record, got status=%q ok=%v (extra=%+v)",
			statusErrUnknownSigner, status, ok, resp.Extra)
	}
	if _, pinned := s.Keys.Get("example.org."); pinned {
		t.Fatalf("expected no key to be pinned for a denied first-contact push")
	}
}

// TestOnboardDeniedForOtherChainReasonsCarriesNoDiagnostic proves the two
// dedicated diagnostics above are specific: a genuinely generic
// chain-of-trust failure (a broken ancestor, a network error -- anything
// that isn't "no DS" or "wrong key") is still refused, but without
// implying either of those more specific, actionable situations when
// neither is actually what happened.
func TestOnboardDeniedForOtherChainReasonsCarriesNoDiagnostic(t *testing.T) {
	s := newTestSazu("example.org.")
	s.InsecureSkipChainValidation = false
	s.Validator = fakeValidator{err: &ChainError{Op: "root-dnskey", Msg: "root did not answer authoritatively for its own DNSKEY"}}
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	push, err := BuildFullZonePush("example.org.", testSOA(1), nil, key, priv, nil)
	if err != nil {
		t.Fatalf("building push: %v", err)
	}
	now := time.Now()
	wire, err := SignUpdate(push, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	resp := sendRaw(t, addr, wire)
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("rcode = %s, want REFUSED", dns.RcodeToString[resp.Rcode])
	}
	if _, ok := diagnosticStatus(resp); ok {
		t.Fatalf("expected no diagnostic TXT record for a generic chain failure, got extra=%+v", resp.Extra)
	}
}

// diagnosticStatus extracts a §12 SAZU status code from a response's
// Additional section, if present.
func diagnosticStatus(m *dns.Msg) (string, bool) {
	for _, rr := range m.Extra {
		if txt, ok := rr.(*dns.TXT); ok && len(txt.Txt) > 0 {
			return txt.Txt[0], true
		}
	}
	return "", false
}

func onboard(t *testing.T, addr, zone string) *dns.DNSKEY {
	t.Helper()
	key, priv, err := GenerateEd25519Key(zone, true)
	if err != nil {
		t.Fatalf("generating key for %s: %v", zone, err)
	}
	soa := synthesizeSOA(zone)
	rrs := []dns.RR{&dns.A{
		Hdr: dns.RR_Header{Name: "www." + dns.Fqdn(zone), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   net.IPv4(203, 0, 113, 10),
	}}
	push, err := BuildFullZonePush(zone, soa, rrs, key, priv, nil)
	if err != nil {
		t.Fatalf("building onboarding push for %s: %v", zone, err)
	}
	now := time.Now()
	wire, err := SignUpdate(push, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing onboarding push for %s: %v", zone, err)
	}
	resp := sendRaw(t, addr, wire)
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("onboarding %s: rcode = %s, want NOERROR", zone, dns.RcodeToString[resp.Rcode])
	}
	return key
}

// TestWildcardScopeOnboardsMultipleDomainsWithoutCorefileChanges is the
// exact scenario a real multi-tenant hoster needs: one Corefile entry
// ("sazu ." -- accept any domain), and every actual zone this instance
// serves comes entirely from what's been onboarded at runtime, with no
// Corefile edit or restart needed per new customer domain. This is the
// regression test for a real bug: zone routing used to conflate "which
// static Corefile entry matched" with "which zone is this request
// about," which under a wildcard "." scope collapsed every distinct
// domain onto the single literal zone ".".
func TestWildcardScopeOnboardsMultipleDomainsWithoutCorefileChanges(t *testing.T) {
	s := &Sazu{
		Zones:                       []string{"."}, // catch-all: accept any domain
		Store:                       NewStore(),
		Keys:                        NewKeyRegistry(),
		Validator:                   NewValidator(),
		Capture:                     NewRawCapture(5*time.Second, 64),
		InsecureSkipChainValidation: true,
	}
	addr := serveThroughRealServer(t, s)

	onboard(t, addr, "first.example.")
	onboard(t, addr, "second.example.")

	firstAnswer := query(t, addr, "www.first.example.", dns.TypeA)
	if len(firstAnswer.Answer) != 1 {
		t.Fatalf("expected first.example.'s own record, got %d answers", len(firstAnswer.Answer))
	}
	secondAnswer := query(t, addr, "www.second.example.", dns.TypeA)
	if len(secondAnswer.Answer) != 1 {
		t.Fatalf("expected second.example.'s own record, got %d answers", len(secondAnswer.Answer))
	}

	if _, ok := s.Keys.Get("first.example."); !ok {
		t.Fatalf("expected first.example. to have its own pinned key, distinct from \".\"")
	}
	if _, ok := s.Keys.Get("second.example."); !ok {
		t.Fatalf("expected second.example. to have its own pinned key, distinct from \".\"")
	}
	if _, ok := s.Keys.Get("."); ok {
		t.Fatalf(`expected no key ever pinned for the literal zone "." itself`)
	}
}

// TestWildcardScopeFallsThroughForNeverOnboardedNames proves a broad
// "sazu ." scope doesn't swallow every query on the server -- only names
// under zones actually onboarded should be answered; anything else must
// fall through to whatever's configured after sazu in the plugin chain
// (here, a plugin that always answers, standing in for e.g. forward/file).
func TestWildcardScopeFallsThroughForNeverOnboardedNames(t *testing.T) {
	s := &Sazu{
		Zones:                       []string{"."},
		Store:                       NewStore(),
		Keys:                        NewKeyRegistry(),
		Validator:                   NewValidator(),
		Capture:                     NewRawCapture(5*time.Second, 64),
		InsecureSkipChainValidation: true,
	}
	fallback := &fallthroughHandler{}
	cfg := &dnsserver.Config{Zone: ".", Transport: "dns", ListenHosts: []string{"127.0.0.1"}, Port: "0"}
	cfg.AddPlugin(func(next plugin.Handler) plugin.Handler {
		fallback.Next = next
		s.Next = fallback
		return s
	})
	cfg.AllowOpcode(dns.OpcodeUpdate)
	cfg.UDPDecorateReaderFunc = s.Capture.DecorateReaderFunc
	cfg.UDPSize = maxUDPMessageSize
	srv, err := dnsserver.NewServer("127.0.0.1:0", []*dnsserver.Config{cfg})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	defer pc.Close()
	go func() { _ = srv.ServePacket(pc) }()
	defer srv.Stop()
	addr := pc.LocalAddr().String()

	onboard(t, addr, "onboarded.example.")

	query(t, addr, "www.never-onboarded.example.", dns.TypeA)
	if !fallback.called.Load() {
		t.Fatalf("expected a query for a never-onboarded name to fall through to the next plugin")
	}
}

// fallthroughHandler always answers, standing in for whatever a real
// deployment would chain after sazu (forward, file, ...).
type fallthroughHandler struct {
	Next   plugin.Handler
	called atomic.Bool
}

func (f *fallthroughHandler) Name() string { return "fallthrough-stub" }

func (f *fallthroughHandler) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	f.called.Store(true)
	m := new(dns.Msg)
	m.SetReply(r)
	if err := w.WriteMsg(m); err != nil {
		return dns.RcodeServerFailure, err
	}
	return dns.RcodeSuccess, nil
}
