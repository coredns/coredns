package sazu

import (
	"crypto/ed25519"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// onboardWithKSK sends a minimal first-contact push for "example.org."
// signed by ksk over a real socket, and fails the test unless it's
// accepted -- shared setup for every ZSK test below.
func onboardWithKSK(t *testing.T, addr string, ksk *dns.DNSKEY, kskPriv ed25519.PrivateKey) {
	t.Helper()
	onboard, err := BuildFullZonePush("example.org.", testSOA(1), nil, ksk, kskPriv, nil)
	if err != nil {
		t.Fatalf("building onboarding push: %v", err)
	}
	now := time.Now()
	wire, err := SignUpdate(onboard, ksk, kskPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing onboarding push: %v", err)
	}
	if resp := sendRaw(t, addr, wire); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("onboarding push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
}

// addZSK sends an ordinary, already-authenticated push (signed by
// authKey, the KSK or an already-authorized ZSK) that adds zsk's DNSKEY
// record at the zone apex -- the cheap registration path, with no chain-
// of-trust network walk, findNewZSKCandidate/AddZSK exist for.
func addZSK(t *testing.T, addr string, authKey *dns.DNSKEY, authPriv ed25519.PrivateKey, zsk *dns.DNSKEY) *dns.Msg {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeSOA)
	m.Opcode = dns.OpcodeUpdate
	m.Insert([]dns.RR{&dns.DNSKEY{
		Hdr:       dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
		Flags:     zsk.Flags,
		Protocol:  zsk.Protocol,
		Algorithm: zsk.Algorithm,
		PublicKey: zsk.PublicKey,
	}})
	now := time.Now()
	wire, err := SignUpdate(m, authKey, authPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing ZSK-add push: %v", err)
	}
	return sendRaw(t, addr, wire)
}

// retireZSK sends an RFC 2136 §2.5.4 "delete one RR" push removing
// zsk's DNSKEY record, authenticated by authKey.
func retireZSK(t *testing.T, addr string, authKey *dns.DNSKEY, authPriv ed25519.PrivateKey, zsk *dns.DNSKEY) *dns.Msg {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeSOA)
	m.Opcode = dns.OpcodeUpdate
	m.Remove([]dns.RR{&dns.DNSKEY{
		Hdr:       dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
		Flags:     zsk.Flags,
		Protocol:  zsk.Protocol,
		Algorithm: zsk.Algorithm,
		PublicKey: zsk.PublicKey,
	}})
	now := time.Now()
	wire, err := SignUpdate(m, authKey, authPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing ZSK-retire push: %v", err)
	}
	return sendRaw(t, addr, wire)
}

// TestZSKCanBeRegisteredOverAnOrdinaryPush proves the cheap registration
// path: a ZSK added via an ordinary, KSK-authenticated push is accepted
// with no chain-of-trust network walk (s.InsecureSkipChainValidation
// stays true throughout -- if this path incorrectly fell into the
// KSK-rollover branch instead, s.Validator would need to be consulted,
// which newTestSazu's default fakeValidator-less Validator would fail
// loudly on).
func TestZSKCanBeRegisteredOverAnOrdinaryPush(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	ksk, kskPriv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating KSK: %v", err)
	}
	onboardWithKSK(t, addr, ksk, kskPriv)

	zsk, _, err := GenerateEd25519Key("example.org.", false)
	if err != nil {
		t.Fatalf("generating ZSK: %v", err)
	}
	resp := addZSK(t, addr, ksk, kskPriv, zsk)
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("ZSK registration rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	zk, ok := s.Keys.Get("example.org.")
	if !ok {
		t.Fatalf("expected zone keys to exist")
	}
	if len(zk.ZSKs) != 1 || zk.ZSKs[0].DNSKEY.PublicKey != zsk.PublicKey {
		t.Fatalf("expected the new ZSK to be registered, got %+v", zk.ZSKs)
	}
	if zk.KSK.DNSKEY.PublicKey != ksk.PublicKey {
		t.Fatalf("expected the KSK to be unaffected by registering a ZSK")
	}
}

// TestRegisteringAZSKIsOptionalZoneBehaviorIsUnchangedWithoutOne proves
// the central design constraint: a zone that never registers a ZSK
// behaves exactly as SAZU always has -- an ordinary partial push signed
// by the KSK alone still works with no ZSK involved anywhere.
func TestRegisteringAZSKIsOptionalZoneBehaviorIsUnchangedWithoutOne(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	ksk, kskPriv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating KSK: %v", err)
	}
	onboardWithKSK(t, addr, ksk, kskPriv)

	partial := new(dns.Msg)
	partial.SetQuestion("example.org.", dns.TypeSOA)
	partial.Opcode = dns.OpcodeUpdate
	partial.Insert([]dns.RR{testA("www.example.org.", net.IPv4(203, 0, 113, 10))})
	now := time.Now()
	wire, err := SignUpdate(partial, ksk, kskPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	if resp := sendRaw(t, addr, wire); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	zk, _ := s.Keys.Get("example.org.")
	if len(zk.ZSKs) != 0 {
		t.Fatalf("expected zero ZSKs on a zone that never registered one, got %d", len(zk.ZSKs))
	}
}

// TestZSKAuthenticatesFurtherTransactionsOnceRegistered proves the
// practical point of the split: once registered, a ZSK's own SIG(0) can
// authenticate later pushes on its own, with no KSK involvement at all.
func TestZSKAuthenticatesFurtherTransactionsOnceRegistered(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	ksk, kskPriv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating KSK: %v", err)
	}
	onboardWithKSK(t, addr, ksk, kskPriv)

	zsk, zskPriv, err := GenerateEd25519Key("example.org.", false)
	if err != nil {
		t.Fatalf("generating ZSK: %v", err)
	}
	if resp := addZSK(t, addr, ksk, kskPriv, zsk); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("ZSK registration rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	// A routine content push, authenticated by the ZSK alone.
	partial := new(dns.Msg)
	partial.SetQuestion("example.org.", dns.TypeSOA)
	partial.Opcode = dns.OpcodeUpdate
	partial.Insert([]dns.RR{testA("www.example.org.", net.IPv4(203, 0, 113, 10))})
	now := time.Now()
	wire, err := SignUpdate(partial, zsk, zskPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	resp := sendRaw(t, addr, wire)
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("ZSK-authenticated push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	answer := query(t, addr, "www.example.org.", dns.TypeA)
	if len(answer.Answer) != 1 {
		t.Fatalf("expected the ZSK-authenticated push's content to be servable, got %d answers", len(answer.Answer))
	}
}

// TestZSKCanSignContentVerifiedUnderRequireValidRRSIGs proves the ZSK is
// a real content-signing key under §4's "Level 2" verification, not just
// a transaction-authentication identity: content signed by the ZSK
// (rather than the KSK) still verifies, because VerifySignedRRsets is
// given the zone's whole current content-signer set, KSK and ZSK alike.
func TestZSKCanSignContentVerifiedUnderRequireValidRRSIGs(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	ksk, kskPriv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating KSK: %v", err)
	}
	onboardWithKSK(t, addr, ksk, kskPriv)

	zsk, zskPriv, err := GenerateEd25519Key("example.org.", false)
	if err != nil {
		t.Fatalf("generating ZSK: %v", err)
	}
	// Registered before RequireValidRRSIGs is turned on below, so this
	// unsigned-content DNSKEY-add push (Level 0 is all onboarding/ZSK
	// registration in this package ever needs -- see AUTH-03's
	// SIG(0)-is-the-authentication-layer design) isn't itself subject to
	// the Level 2 content-signature check this test is actually about.
	if resp := addZSK(t, addr, ksk, kskPriv, zsk); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("ZSK registration rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	s.RequireValidRRSIGs = true

	rrs := []dns.RR{testA("www.example.org.", net.IPv4(203, 0, 113, 10))}
	now := time.Now()
	signed, err := SignZoneContent(rrs, zsk, zskPriv, now.Add(-DefaultSignatureInceptionSkew), now.Add(DefaultSignatureValidity))
	if err != nil {
		t.Fatalf("signing content with the ZSK: %v", err)
	}
	partial := new(dns.Msg)
	partial.SetQuestion("example.org.", dns.TypeSOA)
	partial.Opcode = dns.OpcodeUpdate
	partial.Insert(signed)
	wire, err := SignUpdate(partial, ksk, kskPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing transaction: %v", err)
	}
	resp := sendRaw(t, addr, wire)
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s, want NOERROR (content signed by a registered ZSK should verify)", dns.RcodeToString[resp.Rcode])
	}
}

// TestZSKCanBeRetired proves the reverse of registration: retiring a ZSK
// removes it from the zone's trusted key set, and it no longer
// authenticates anything afterward.
func TestZSKCanBeRetired(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	ksk, kskPriv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating KSK: %v", err)
	}
	onboardWithKSK(t, addr, ksk, kskPriv)

	zsk, zskPriv, err := GenerateEd25519Key("example.org.", false)
	if err != nil {
		t.Fatalf("generating ZSK: %v", err)
	}
	if resp := addZSK(t, addr, ksk, kskPriv, zsk); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("ZSK registration rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	if resp := retireZSK(t, addr, ksk, kskPriv, zsk); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("ZSK retirement rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	zk, _ := s.Keys.Get("example.org.")
	if len(zk.ZSKs) != 0 {
		t.Fatalf("expected no ZSKs left after retiring the only one, got %+v", zk.ZSKs)
	}

	// The retired ZSK's own signature no longer authenticates anything.
	partial := new(dns.Msg)
	partial.SetQuestion("example.org.", dns.TypeSOA)
	partial.Opcode = dns.OpcodeUpdate
	partial.Insert([]dns.RR{testA("www.example.org.", net.IPv4(203, 0, 113, 10))})
	now := time.Now()
	wire, err := SignUpdate(partial, zsk, zskPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	resp := sendRaw(t, addr, wire)
	if resp.Rcode == dns.RcodeSuccess {
		t.Fatalf("expected a push authenticated by a retired ZSK to be rejected")
	}
}

// TestKSKRolloverPreservesExistingZSK proves KeyRegistry.PinKSK's own
// documented guarantee end to end: rolling the KSK over does not
// invalidate a ZSK registered under the old one -- it keeps
// authenticating transactions immediately afterward, with no re-
// registration needed.
func TestKSKRolloverPreservesExistingZSK(t *testing.T) {
	s := newTestSazu("example.org.")
	s.InsecureSkipChainValidation = false
	s.Validator = fakeValidator{err: nil} // the new KSK's DS "checks out"
	addr := serveThroughRealServer(t, s)

	oldKSK, oldKSKPriv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating old KSK: %v", err)
	}
	onboardWithKSK(t, addr, oldKSK, oldKSKPriv)

	zsk, zskPriv, err := GenerateEd25519Key("example.org.", false)
	if err != nil {
		t.Fatalf("generating ZSK: %v", err)
	}
	if resp := addZSK(t, addr, oldKSK, oldKSKPriv, zsk); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("ZSK registration rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	newKSK, newKSKPriv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating new KSK: %v", err)
	}
	rollover := new(dns.Msg)
	rollover.SetQuestion("example.org.", dns.TypeSOA)
	rollover.Opcode = dns.OpcodeUpdate
	rollover.Insert([]dns.RR{&dns.DNSKEY{
		Hdr:   dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
		Flags: newKSK.Flags, Protocol: newKSK.Protocol, Algorithm: newKSK.Algorithm, PublicKey: newKSK.PublicKey,
	}})
	now := time.Now()
	rolloverWire, err := SignUpdate(rollover, newKSK, newKSKPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing rollover push: %v", err)
	}
	if resp := sendRaw(t, addr, rolloverWire); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rollover push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	zk, ok := s.Keys.Get("example.org.")
	if !ok || zk.KSK.DNSKEY.PublicKey != newKSK.PublicKey {
		t.Fatalf("expected the KSK to have rolled over")
	}
	if len(zk.ZSKs) != 1 || zk.ZSKs[0].DNSKEY.PublicKey != zsk.PublicKey {
		t.Fatalf("expected the ZSK to survive the KSK rollover untouched, got %+v", zk.ZSKs)
	}

	// The ZSK, registered under the now-superseded KSK, still
	// authenticates a push on its own.
	partial := new(dns.Msg)
	partial.SetQuestion("example.org.", dns.TypeSOA)
	partial.Opcode = dns.OpcodeUpdate
	partial.Insert([]dns.RR{testA("www.example.org.", net.IPv4(203, 0, 113, 10))})
	now = time.Now()
	wire, err := SignUpdate(partial, zsk, zskPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	resp := sendRaw(t, addr, wire)
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("post-rollover ZSK-authenticated push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
}

// TestFirstContactRejectsNonKSKCandidate proves a zone can only ever be
// established (first contact) by a SEP-flagged (KSK) key -- a candidate
// presented without that flag is refused with a specific diagnostic, and
// pins nothing.
func TestFirstContactRejectsNonKSKCandidate(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	zskShaped, zskPriv, err := GenerateEd25519Key("example.org.", false)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	onboard, err := BuildFullZonePush("example.org.", testSOA(1), nil, zskShaped, zskPriv, nil)
	if err != nil {
		t.Fatalf("building push: %v", err)
	}
	now := time.Now()
	wire, err := SignUpdate(onboard, zskShaped, zskPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	resp := sendRaw(t, addr, wire)
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("rcode = %s, want REFUSED", dns.RcodeToString[resp.Rcode])
	}
	if status, ok := diagnosticStatus(resp); !ok || status != statusErrFirstContactNeedsKSK {
		t.Fatalf("expected %s diagnostic, got status=%q ok=%v", statusErrFirstContactNeedsKSK, status, ok)
	}
	if _, pinned := s.Keys.Get("example.org."); pinned {
		t.Fatalf("expected no key to be pinned for a rejected first-contact push")
	}
}

// TestAddZSKRejectsWeakAlgorithm proves §10.7's algorithm floor applies
// to a ZSK registration exactly as it does at first contact and KSK
// rollover -- checked before the key is registered.
func TestAddZSKRejectsWeakAlgorithm(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	ksk, kskPriv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating KSK: %v", err)
	}
	onboardWithKSK(t, addr, ksk, kskPriv)

	weakZSK := &dns.DNSKEY{
		Hdr:       dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
		Flags:     dns.ZONE, // ZSK-shaped, no SEP
		Protocol:  3,
		Algorithm: dns.RSAMD5, // well below the floor
		PublicKey: "AAAA",
	}
	resp := addZSK(t, addr, ksk, kskPriv, weakZSK)
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("rcode = %s, want REFUSED", dns.RcodeToString[resp.Rcode])
	}
	if status, ok := diagnosticStatus(resp); !ok || status != statusErrWeakAlgorithm {
		t.Fatalf("expected %s diagnostic, got status=%q ok=%v", statusErrWeakAlgorithm, status, ok)
	}

	zk, _ := s.Keys.Get("example.org.")
	if len(zk.ZSKs) != 0 {
		t.Fatalf("expected the weak-algorithm ZSK to not be registered, got %+v", zk.ZSKs)
	}
}
