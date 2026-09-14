package sazu

import (
	"context"
	"crypto/ed25519"
	"net"
	"testing"
	"time"

	"github.com/coredns/coredns/core/dnsserver"

	"github.com/miekg/dns"
)

// fakeAddr is a minimal net.Addr for exercising connectionOriented
// directly, without needing a real socket of the given network type.
type fakeAddr struct{ network string }

func (a fakeAddr) Network() string { return a.network }
func (a fakeAddr) String() string  { return "test-addr" }

// fakeResponseWriterAddr is just enough of dns.ResponseWriter for
// connectionOriented, which only ever calls RemoteAddr() on it.
type fakeResponseWriterAddr struct {
	dns.ResponseWriter
	addr net.Addr
}

func (w fakeResponseWriterAddr) RemoteAddr() net.Addr { return w.addr }

func TestConnectionOrientedUDPIsNotConnectionOriented(t *testing.T) {
	w := fakeResponseWriterAddr{addr: fakeAddr{network: "udp"}}
	if connectionOriented(context.Background(), w) {
		t.Fatalf("expected a plain UDP address, with no RawRequestKey, to be reported as NOT connection-oriented")
	}
}

func TestConnectionOrientedTCPIsConnectionOriented(t *testing.T) {
	w := fakeResponseWriterAddr{addr: fakeAddr{network: "tcp"}}
	if !connectionOriented(context.Background(), w) {
		t.Fatalf("expected a TCP address to be reported as connection-oriented")
	}
}

// TestConnectionOrientedHTTP3ShapedUDPAddrIsStillConnectionOriented
// proves the specific reason connectionOriented checks RawRequestKey
// first, rather than relying on RemoteAddr()'s network type alone:
// ServerHTTPS3 constructs its DoHWriter's RemoteAddr as a *net.UDPAddr
// (QUIC itself runs over UDP), which would otherwise be indistinguishable
// from plain, spoofable UDP -- even though QUIC's own handshake makes an
// HTTP/3 request just as address-validated as TCP.
func TestConnectionOrientedHTTP3ShapedUDPAddrIsStillConnectionOriented(t *testing.T) {
	w := fakeResponseWriterAddr{addr: fakeAddr{network: "udp"}}
	ctx := context.WithValue(context.Background(), dnsserver.RawRequestKey{}, []byte("wire bytes"))
	if !connectionOriented(ctx, w) {
		t.Fatalf("expected RawRequestKey's presence to mark this as connection-oriented, regardless of the UDP-shaped RemoteAddr")
	}
}

func TestConnectionOrientedNilAddrIsNotConnectionOriented(t *testing.T) {
	w := fakeResponseWriterAddr{addr: nil}
	if connectionOriented(context.Background(), w) {
		t.Fatalf("expected a nil RemoteAddr, with no RawRequestKey, to be reported as NOT connection-oriented")
	}
}

// sendRawUDP sends wire to addr over plain UDP (no length-prefix
// framing, matching a raw DNS-over-UDP datagram) and returns the
// response -- unlike sendRaw (this package's other tests), which always
// uses TCP. Exists specifically to exercise the one code path that
// deliberately behaves differently over UDP: connectionOriented's
// first-contact/rollover transport gate.
func sendRawUDP(t *testing.T, addr string, wire []byte) *dns.Msg {
	t.Helper()
	conn, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}
	if _, err := conn.Write(wire); err != nil {
		t.Fatalf("Write: %v", err)
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

// minimalFirstContactWire builds the smallest push that actually reaches
// serveUpdate's chain-of-trust walk: a candidate DNSKEY plus a genuine
// SIG(0) signature over it, and nothing else at all -- no SOA, no RRSIGs,
// no NSEC chain. containsAPEXSOA is only checked *after* the chain-of-
// trust walk (see serveUpdate), so a real attacker mounting exactly the
// flood this test's gate defends against has no reason to send anything
// bigger than this: every extra byte only costs them packets-per-second,
// buying nothing. This is deliberately smaller than a realistic
// legitimate push (which always includes at least a SOA and, from a full
// push, an NSEC chain and RRSIGs -- routinely well over 512 bytes, as
// found the hard way testing this package against real UDP paths
// elsewhere) specifically so the test exercises connectionOriented's own
// gate rather than incidentally tripping RFC 1035's unrelated 512-byte
// UDP truncation limit first.
func minimalFirstContactWire(t *testing.T, zone string, key *dns.DNSKEY, priv ed25519.PrivateKey) []byte {
	t.Helper()
	m := new(dns.Msg)
	m.SetUpdate(dns.Fqdn(zone))
	m.Insert([]dns.RR{key})
	now := time.Now()
	wire, err := SignUpdate(m, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing minimal first-contact push: %v", err)
	}
	if len(wire) > 512 {
		t.Fatalf("minimal first-contact push unexpectedly exceeds a single safe UDP datagram (%d bytes)", len(wire))
	}
	return wire
}

// TestFirstContactOverUDPIsRefused proves the transport gate end to end
// against exactly the packet a real attacker would actually send: the
// smallest message that still reaches the chain-of-trust walk (see
// minimalFirstContactWire). Refused specifically because plain UDP lets
// an attacker forge any source address on a single packet with nothing
// to disprove it -- exactly the property that would otherwise let
// IPRateLimiter's per-address budget be bypassed by varying the
// (spoofed) source address on every attempt, each one still triggering
// a real outbound chain-of-trust network walk.
func TestFirstContactOverUDPIsRefused(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	wire := minimalFirstContactWire(t, "example.org.", key, priv)

	resp := sendRawUDP(t, addr, wire)
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("first-contact-over-UDP rcode = %s, want REFUSED", dns.RcodeToString[resp.Rcode])
	}
	if status, ok := diagnosticStatus(resp); !ok || status != statusErrTransportNotAllowed {
		t.Fatalf("expected %s diagnostic, got status=%q ok=%v", statusErrTransportNotAllowed, status, ok)
	}
	if _, ok := s.Keys.Get("example.org."); ok {
		t.Fatalf("expected no key to be pinned for a first-contact attempt over UDP")
	}
}

// TestFirstContactOverUDPStillWorksAfterRetryOverTCP proves the gate
// only ever blocks the UDP attempt itself -- exactly the same push,
// resent over TCP, succeeds normally (once it also carries a real SOA,
// which containsAPEXSOA still separately requires -- the minimal wire
// above deliberately omits one, since a real attacker triggering the
// network walk has no reason to include it either).
func TestFirstContactOverUDPStillWorksAfterRetryOverTCP(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	udpWire := minimalFirstContactWire(t, "example.org.", key, priv)
	if resp := sendRawUDP(t, addr, udpWire); resp.Rcode != dns.RcodeRefused {
		t.Fatalf("expected the UDP attempt to be refused first, got %s", dns.RcodeToString[resp.Rcode])
	}

	push, err := BuildFullZonePush("example.org.", testSOA(1), nil, key, priv, nil)
	if err != nil {
		t.Fatalf("building push: %v", err)
	}
	now := time.Now()
	tcpWire, err := SignUpdate(push, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	if resp := sendRaw(t, addr, tcpWire); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("TCP retry rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if _, ok := s.Keys.Get("example.org."); !ok {
		t.Fatalf("expected the zone to be onboarded after the TCP retry")
	}
}

// TestKeyRolloverOverUDPIsRefused proves the gate also covers rollover,
// not just first contact -- the other branch that triggers a real
// chain-of-trust network walk.
func TestKeyRolloverOverUDPIsRefused(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)
	oldKey, oldPriv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating old key: %v", err)
	}
	onboardPush, err := BuildFullZonePush("example.org.", testSOA(1), nil, oldKey, oldPriv, nil)
	if err != nil {
		t.Fatalf("building onboarding push: %v", err)
	}
	now := time.Now()
	onboardWire, err := SignUpdate(onboardPush, oldKey, oldPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing onboarding push: %v", err)
	}
	if resp := sendRaw(t, addr, onboardWire); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("onboarding push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	newKey, newPriv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating new key: %v", err)
	}
	rollover := new(dns.Msg)
	rollover.SetQuestion("example.org.", dns.TypeSOA)
	rollover.Opcode = dns.OpcodeUpdate
	rollover.Insert([]dns.RR{
		&dns.DNSKEY{Hdr: dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
			Flags: newKey.Flags, Protocol: newKey.Protocol, Algorithm: newKey.Algorithm, PublicKey: newKey.PublicKey},
	})
	now = time.Now()
	rolloverWire, err := SignUpdate(rollover, newKey, newPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing rollover push: %v", err)
	}

	resp := sendRawUDP(t, addr, rolloverWire)
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("rollover-over-UDP rcode = %s, want REFUSED", dns.RcodeToString[resp.Rcode])
	}
	if status, ok := diagnosticStatus(resp); !ok || status != statusErrTransportNotAllowed {
		t.Fatalf("expected %s diagnostic, got status=%q ok=%v", statusErrTransportNotAllowed, status, ok)
	}
	if pinned, ok := s.Keys.Get("example.org."); !ok || pinned.PublicKey != oldKey.PublicKey {
		t.Fatalf("expected the zone to remain pinned to the old key after a rejected UDP rollover attempt")
	}
}

// TestOrdinaryPartialPushOverUDPStillWorks proves the gate is scoped to
// first-contact/rollover specifically -- an ordinary push to an
// already-pinned zone (which never touches the chain-of-trust walk this
// gate protects) still works over UDP exactly as before.
func TestOrdinaryPartialPushOverUDPStillWorks(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	onboardPush, err := BuildFullZonePush("example.org.", testSOA(1), nil, key, priv, nil)
	if err != nil {
		t.Fatalf("building onboarding push: %v", err)
	}
	now := time.Now()
	onboardWire, err := SignUpdate(onboardPush, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing onboarding push: %v", err)
	}
	if resp := sendRaw(t, addr, onboardWire); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("onboarding push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	partial := new(dns.Msg)
	partial.SetUpdate("example.org.")
	partial.Insert([]dns.RR{testA("mail.example.org.", net.IPv4(203, 0, 113, 20))})
	now = time.Now()
	partialWire, err := SignUpdate(partial, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing partial push: %v", err)
	}
	resp := sendRawUDP(t, addr, partialWire)
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("ordinary partial push over UDP rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
}
