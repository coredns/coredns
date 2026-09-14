package sazu

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"io"
	"net"
	"strings"
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
		Contacts:                    NewContactRegistry(),
		Validator:                   NewValidator(),
		Capture:                     NewRawCapture(5*time.Second, 64),
		InsecureSkipChainValidation: true,
	}
}

// serveThroughRealServer starts a real dnsserver.Server (as CoreDNS
// itself would) with s installed as the sole plugin, listening on both
// UDP and TCP on the same port -- exactly like a real deployment -- so
// its DecorateReaderFunc-based raw capture is genuinely exercised over a
// real round trip on either transport, not called directly, which would
// prove nothing about the wiring this depends on.
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
	cfg.TCPDecorateReaderFunc = s.Capture.DecorateReaderFunc

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

	// Bind TCP to the exact same port UDP got assigned, matching a real
	// CoreDNS deployment (one address, both transports).
	_, port, err := net.SplitHostPort(pc.LocalAddr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	go func() { _ = srv.Serve(l) }()

	t.Cleanup(func() { _ = srv.Stop() })
	return pc.LocalAddr().String()
}

// sendRaw sends wire directly over TCP to addr (RFC 1035 §4.2.2 length-
// prefix framing) and returns the response, bypassing dns.Client/
// dns.Exchange -- which would re-pack the message and defeat the entire
// point of testing byte-exact SIG(0) delivery. TCP, not UDP, because a
// real signed push routinely exceeds the ~1472-byte path MTU and gets
// silently dropped as an IP fragment on real networks -- exactly what
// sazuctl itself now avoids by using TCP for anything of meaningful size
// (see push.go); UDP-specific behavior has its own dedicated tests in
// rawcapture_test.go.
func sendRaw(t *testing.T, addr string, wire []byte) *dns.Msg {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}
	var lenPrefix [2]byte
	binary.BigEndian.PutUint16(lenPrefix[:], uint16(len(wire)))
	if _, err := conn.Write(lenPrefix[:]); err != nil {
		t.Fatalf("writing length prefix: %v", err)
	}
	if _, err := conn.Write(wire); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if _, err := io.ReadFull(conn, lenPrefix[:]); err != nil {
		t.Fatalf("reading response length prefix: %v", err)
	}
	buf := make([]byte, binary.BigEndian.Uint16(lenPrefix[:]))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("reading response: %v", err)
	}
	resp := new(dns.Msg)
	if err := resp.Unpack(buf); err != nil {
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

// TestNXDOMAINCarriesSOAInAuthority and TestNODATACarriesSOAInAuthority
// prove RFC 2308 §3: any negative response -- NXDOMAIN (name doesn't
// exist at all) or NODATA (name exists, just not this type) -- carries
// the zone's SOA in the authority section, which a resolver needs to
// know how long it may cache the negative result for. A real gap found
// comparing this server's answers directly against a real, standards-
// compliant authoritative server (AWS Route 53) for the same zone: AWS's
// negative answers carried SOA+RRSIG(SOA)+NSEC+RRSIG(NSEC) in Authority;
// this server's carried nothing there at all.
func TestNXDOMAINCarriesSOAInAuthority(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)
	onboardExampleOrg(t, addr, s)

	resp := query(t, addr, "does-not-exist.example.org.", dns.TypeA)
	if resp.Rcode != dns.RcodeNameError {
		t.Fatalf("rcode = %s, want NXDOMAIN", dns.RcodeToString[resp.Rcode])
	}
	soa := soaFromAuthority(t, resp)
	if soa.Serial != 1 {
		t.Fatalf("expected the zone's real SOA (serial 1) in authority, got %+v", soa)
	}
}

func TestNODATACarriesSOAInAuthority(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)
	onboardExampleOrg(t, addr, s)

	// www.example.org. exists (has an A record) but has no TXT record --
	// NODATA, not NXDOMAIN.
	resp := query(t, addr, "www.example.org.", dns.TypeTXT)
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s, want NOERROR (NODATA)", dns.RcodeToString[resp.Rcode])
	}
	if len(resp.Answer) != 0 {
		t.Fatalf("expected no answers for a NODATA response, got %+v", resp.Answer)
	}
	soa := soaFromAuthority(t, resp)
	if soa.Serial != 1 {
		t.Fatalf("expected the zone's real SOA (serial 1) in authority, got %+v", soa)
	}
}

// TestNegativeResponseCarriesSOARRSIGWithDOBit proves the authority-
// section SOA is itself signed when the query asked for DNSSEC, the same
// way a positive answer's RRset is.
func TestNegativeResponseCarriesSOARRSIGWithDOBit(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)
	key := onboardExampleOrg(t, addr, s)

	resp := queryDO(t, addr, "does-not-exist.example.org.", dns.TypeA)
	var soa *dns.SOA
	var sig *dns.RRSIG
	for _, rr := range resp.Ns {
		switch v := rr.(type) {
		case *dns.SOA:
			soa = v
		case *dns.RRSIG:
			if v.TypeCovered == dns.TypeSOA {
				sig = v
			}
		}
	}
	if soa == nil || sig == nil {
		t.Fatalf("expected both SOA and its covering RRSIG in authority with DO set, got %+v", resp.Ns)
	}
	if err := sig.Verify(key, []dns.RR{soa}); err != nil {
		t.Fatalf("the served authority-section RRSIG does not verify: %v", err)
	}
}

// TestNXDOMAINCarriesValidNSECProof is the real end-to-end proof this
// whole feature exists for: a full push (via BuildFullZonePush, which
// now synthesizes and signs a real NSEC chain automatically) genuinely
// authenticates an NXDOMAIN answer over the wire, not just in unit tests
// against ZoneData directly.
func TestNXDOMAINCarriesValidNSECProof(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)
	key := onboardExampleOrg(t, addr, s)

	resp := queryDO(t, addr, "does-not-exist.example.org.", dns.TypeA)
	if resp.Rcode != dns.RcodeNameError {
		t.Fatalf("rcode = %s, want NXDOMAIN", dns.RcodeToString[resp.Rcode])
	}
	nsecs, sigs := splitNSECAndRRSIGs(t, resp.Ns)
	if len(nsecs) == 0 {
		t.Fatalf("expected at least one NSEC in authority, got %+v", resp.Ns)
	}
	for _, n := range nsecs {
		sig, ok := sigs[strings.ToLower(n.Hdr.Name)]
		if !ok {
			t.Fatalf("NSEC at %s has no covering RRSIG in the response", n.Hdr.Name)
		}
		if err := sig.Verify(key, []dns.RR{n}); err != nil {
			t.Fatalf("NSEC at %s's RRSIG does not verify: %v", n.Hdr.Name, err)
		}
	}
}

// TestNODATACarriesValidNSECProof mirrors the above for a NODATA answer
// (name exists, queried type doesn't): the NSEC stored at that exact
// name is what's served, and it genuinely verifies.
func TestNODATACarriesValidNSECProof(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)
	key := onboardExampleOrg(t, addr, s)

	resp := queryDO(t, addr, "www.example.org.", dns.TypeTXT)
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 0 {
		t.Fatalf("expected NOERROR/NODATA, got rcode=%s answer=%+v", dns.RcodeToString[resp.Rcode], resp.Answer)
	}
	nsecs, sigs := splitNSECAndRRSIGs(t, resp.Ns)
	if len(nsecs) != 1 || nsecs[0].Hdr.Name != "www.example.org." {
		t.Fatalf("expected exactly one NSEC, at www.example.org., got %+v", nsecs)
	}
	sig, ok := sigs["www.example.org."]
	if !ok {
		t.Fatalf("expected a covering RRSIG for www's NSEC, got %+v", resp.Ns)
	}
	if err := sig.Verify(key, []dns.RR{nsecs[0]}); err != nil {
		t.Fatalf("www's NSEC RRSIG does not verify: %v", err)
	}
}

// TestPartialPushInvalidatesNSECUntilNextFullPush proves the documented
// trade-off (see ZoneData.PurgeNSEC): a partial push, which never
// includes NSEC records of its own, invalidates any existing chain
// rather than risk it going stale -- an NXDOMAIN answer right afterward
// carries no NSEC at all -- and a subsequent full push, which always
// recomputes the whole chain, restores it.
func TestPartialPushInvalidatesNSECUntilNextFullPush(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	soa := testSOA(1)
	rrs := []dns.RR{testA("www.example.org.", net.IPv4(203, 0, 113, 10))}
	fullPush := func() {
		t.Helper()
		push, err := BuildFullZonePush("example.org.", soa, rrs, key, priv, nil)
		if err != nil {
			t.Fatalf("building full push: %v", err)
		}
		now := time.Now()
		wire, err := SignUpdate(push, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
		if err != nil {
			t.Fatalf("signing full push: %v", err)
		}
		if resp := sendRaw(t, addr, wire); resp.Rcode != dns.RcodeSuccess {
			t.Fatalf("full push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
		}
	}
	fullPush() // onboards the zone

	before := queryDO(t, addr, "does-not-exist.example.org.", dns.TypeA)
	if nsecs, _ := splitNSECAndRRSIGs(t, before.Ns); len(nsecs) == 0 {
		t.Fatalf("expected a full push to leave a usable NSEC chain, got none")
	}

	now := time.Now()
	update := new(dns.Msg)
	update.SetUpdate("example.org.")
	update.Insert([]dns.RR{testA("mail.example.org.", net.IPv4(203, 0, 113, 20))})
	wire, err := SignUpdate(update, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing partial update: %v", err)
	}
	if resp := sendRaw(t, addr, wire); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("partial push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	during := queryDO(t, addr, "does-not-exist.example.org.", dns.TypeA)
	if nsecs, _ := splitNSECAndRRSIGs(t, during.Ns); len(nsecs) != 0 {
		t.Fatalf("expected the partial push to invalidate the NSEC chain, still got %+v", nsecs)
	}

	fullPush() // recomputes the chain from the same rrs given to BuildFullZonePush

	after := queryDO(t, addr, "does-not-exist.example.org.", dns.TypeA)
	if nsecs, _ := splitNSECAndRRSIGs(t, after.Ns); len(nsecs) == 0 {
		t.Fatalf("expected a subsequent full push to restore the NSEC chain, got none")
	}
}

// splitNSECAndRRSIGs separates rrs into NSEC records and a map of
// covering-name -> RRSIG-over-NSEC, for tests that need to pair each
// NSEC with its own signature.
func splitNSECAndRRSIGs(t *testing.T, rrs []dns.RR) ([]*dns.NSEC, map[string]*dns.RRSIG) {
	t.Helper()
	var nsecs []*dns.NSEC
	sigs := make(map[string]*dns.RRSIG)
	for _, rr := range rrs {
		switch v := rr.(type) {
		case *dns.NSEC:
			nsecs = append(nsecs, v)
		case *dns.RRSIG:
			if v.TypeCovered == dns.TypeNSEC {
				sigs[strings.ToLower(v.Hdr.Name)] = v
			}
		}
	}
	return nsecs, sigs
}

// onboardExampleOrg onboards example.org. with SOA serial 1 and a single
// www A record, and returns the onboarded key.
func onboardExampleOrg(t *testing.T, addr string, s *Sazu) *dns.DNSKEY {
	t.Helper()
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
	return key
}

// soaFromAuthority extracts the (sole) SOA record from resp's authority
// section, failing the test if it's missing.
func soaFromAuthority(t *testing.T, resp *dns.Msg) *dns.SOA {
	t.Helper()
	for _, rr := range resp.Ns {
		if soa, ok := rr.(*dns.SOA); ok {
			return soa
		}
	}
	t.Fatalf("expected a SOA record in the authority section, got %+v", resp.Ns)
	return nil
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
	cfg.TCPDecorateReaderFunc = s.Capture.DecorateReaderFunc
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
	_, port, err := net.SplitHostPort(pc.LocalAddr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()
	go func() { _ = srv.Serve(l) }()
	defer srv.Stop()
	addr := pc.LocalAddr().String()

	onboard(t, addr, "onboarded.example.")

	query(t, addr, "www.never-onboarded.example.", dns.TypeA)
	if !fallback.called.Load() {
		t.Fatalf("expected a query for a never-onboarded name to fall through to the next plugin")
	}
}

// TestContactRegistrationRidesOrdinaryPushAndIsNeverServed proves §10.6's
// registration record: a contact address travels inside an otherwise
// ordinary, already-authenticated push (no separate protocol/transport of
// its own), ends up in s.Contacts, and -- unlike a DNSKEY -- is never
// itself servable DNS content, since a customer's contact address has no
// reason to be public.
func TestContactRegistrationRidesOrdinaryPushAndIsNeverServed(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	soa := testSOA(1)
	rrs := []dns.RR{testA("www.example.org.", net.IPv4(203, 0, 113, 10))}
	onboardPush, err := BuildFullZonePush("example.org.", soa, rrs, key, priv, nil)
	if err != nil {
		t.Fatalf("building onboarding push: %v", err)
	}
	now := time.Now()
	wire, err := SignUpdate(onboardPush, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing onboarding push: %v", err)
	}
	if resp := sendRaw(t, addr, wire); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("onboarding push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	// A follow-up push carrying only a contact registration -- no zone
	// content at all.
	contactPush := new(dns.Msg)
	contactPush.SetQuestion("example.org.", dns.TypeSOA)
	contactPush.Opcode = dns.OpcodeUpdate
	contactPush.Insert([]dns.RR{contactTXT("example.org.", dns.ClassINET, "mailto:ops@example.org", "https://hooks.example.org/sazu")})
	now = time.Now()
	contactWire, err := SignUpdate(contactPush, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing contact push: %v", err)
	}
	resp := sendRaw(t, addr, contactWire)
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("contact push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	got, ok := s.Contacts.Get("example.org.")
	if !ok || len(got) != 2 {
		t.Fatalf("expected the pushed contact addresses registered, got %+v ok=%v", got, ok)
	}

	// Never itself servable: unlike zone content, no amount of DNSSEC (DO
	// bit or otherwise) should make this queryable.
	answer := queryDO(t, addr, contactOwnerName("example.org."), dns.TypeTXT)
	if answer.Rcode != dns.RcodeNameError || len(answer.Answer) != 0 {
		t.Fatalf("expected the contact record to be unservable (NXDOMAIN), got rcode=%s answer=%+v",
			dns.RcodeToString[answer.Rcode], answer.Answer)
	}

	// A delete-shaped push clears it.
	clearPush := new(dns.Msg)
	clearPush.SetQuestion("example.org.", dns.TypeSOA)
	clearPush.Opcode = dns.OpcodeUpdate
	clearPush.Remove([]dns.RR{&dns.TXT{Hdr: dns.RR_Header{Name: contactOwnerName("example.org."), Rrtype: dns.TypeTXT, Class: dns.ClassINET}}})
	now = time.Now()
	clearWire, err := SignUpdate(clearPush, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing clearing push: %v", err)
	}
	if resp := sendRaw(t, addr, clearWire); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("clearing push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if _, ok := s.Contacts.Get("example.org."); ok {
		t.Fatalf("expected the contact registration to be cleared")
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
