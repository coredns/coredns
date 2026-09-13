package sazu

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func sampleUpdate(zone string) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(zone), dns.TypeSOA) // zone section, RFC 2136 §2.3
	m.Opcode = dns.OpcodeUpdate
	a := &dns.A{
		Hdr: dns.RR_Header{Name: "www." + dns.Fqdn(zone), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   net.IPv4(203, 0, 113, 10),
	}
	m.Insert([]dns.RR{a})
	return m
}

func TestSignThenVerifySIG0Succeeds(t *testing.T) {
	key, priv, err := GenerateEd25519Key("client.example.org.", false)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	m := sampleUpdate("example.org.")
	now := time.Now()
	wire, err := SignUpdate(m, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	if err := VerifySIG0(wire, key); err != nil {
		t.Fatalf("expected genuine SIG(0) to verify, got: %v", err)
	}
}

func TestTamperedMessageFailsSIG0Verification(t *testing.T) {
	key, priv, err := GenerateEd25519Key("client.example.org.", false)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	m := sampleUpdate("example.org.")
	now := time.Now()
	wire, err := SignUpdate(m, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	// Flip the last byte of the signed A record's IP address -- a fixed
	// content field, not a length prefix or name-compression pointer, so
	// the flip is guaranteed to land in pure signed data rather than break
	// parsing before signature comparison ever runs.
	needle := []byte{203, 0, 113, 10}
	pos := -1
	for i := 0; i+len(needle) <= len(wire); i++ {
		match := true
		for j, b := range needle {
			if wire[i+j] != b {
				match = false
				break
			}
		}
		if match {
			pos = i
			break
		}
	}
	if pos == -1 {
		t.Fatalf("could not locate A record IP bytes in wire message")
	}
	wire[pos+len(needle)-1] ^= 0xFF

	if err := VerifySIG0(wire, key); err == nil {
		t.Fatalf("expected tampered message to fail verification")
	}
}

func TestWrongKeyFailsSIG0Verification(t *testing.T) {
	key, priv, err := GenerateEd25519Key("client.example.org.", false)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	otherKey, _, err := GenerateEd25519Key("client.example.org.", false)
	if err != nil {
		t.Fatalf("generating other key: %v", err)
	}
	m := sampleUpdate("example.org.")
	now := time.Now()
	wire, err := SignUpdate(m, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	if err := VerifySIG0(wire, otherKey); err == nil {
		t.Fatalf("expected verification against the wrong key to fail")
	}
}

func TestExpiredSIG0Rejected(t *testing.T) {
	key, priv, err := GenerateEd25519Key("client.example.org.", false)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	m := sampleUpdate("example.org.")
	now := time.Now()
	wire, err := SignUpdate(m, key, priv, now.Add(-2*time.Hour), now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	if err := VerifySIG0(wire, key); err == nil {
		t.Fatalf("expected an already-expired signature to be rejected")
	}
}

func TestNotYetValidSIG0Rejected(t *testing.T) {
	key, priv, err := GenerateEd25519Key("client.example.org.", false)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	m := sampleUpdate("example.org.")
	now := time.Now()
	wire, err := SignUpdate(m, key, priv, now.Add(time.Hour), now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	if err := VerifySIG0(wire, key); err == nil {
		t.Fatalf("expected a not-yet-valid signature to be rejected")
	}
}

func TestMissingSIG0Rejected(t *testing.T) {
	key, _, err := GenerateEd25519Key("client.example.org.", false)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	m := sampleUpdate("example.org.")
	wire, err := m.Pack()
	if err != nil {
		t.Fatalf("packing unsigned message: %v", err)
	}

	if err := VerifySIG0(wire, key); err == nil {
		t.Fatalf("expected a message with no SIG(0) record to be rejected")
	}
}
