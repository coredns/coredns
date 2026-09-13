package sazu

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/miekg/dns"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sazu.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestDBCommitUpdateThenLoadAllReproducesOnboarding(t *testing.T) {
	db := openTestDB(t)

	key, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	ops := []dns.RR{
		&dns.DNSKEY{Hdr: dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
			Flags: key.Flags, Protocol: key.Protocol, Algorithm: key.Algorithm, PublicKey: key.PublicKey},
		testSOA(1),
		testA("www.example.org.", net.IPv4(203, 0, 113, 10)),
	}
	// Mirror ApplyUpdateOps' expectation: these are Add ops, class = zone class.
	for _, rr := range ops {
		rr.Header().Class = dns.ClassINET
	}

	if err := db.CommitUpdate("example.org.", key, ops, dns.ClassINET); err != nil {
		t.Fatalf("CommitUpdate: %v", err)
	}

	store, keys, err := db.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	pinned, ok := keys.Get("example.org.")
	if !ok || pinned.PublicKey != key.PublicKey {
		t.Fatalf("expected the pinned key to survive a round trip")
	}

	z, ok := store.Get("example.org.")
	if !ok {
		t.Fatalf("expected the zone to exist after loading")
	}
	if soa := z.SOA(); soa == nil || soa.Serial != 1 {
		t.Fatalf("expected SOA serial 1 to survive a round trip, got %+v", soa)
	}
	got := z.Lookup("www.example.org.", dns.TypeA)
	if len(got) != 1 || !got[0].(*dns.A).A.Equal(net.IPv4(203, 0, 113, 10)) {
		t.Fatalf("expected the A record to survive a round trip, got %+v", got)
	}
}

func TestDBPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sazu.db")

	db1, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	key, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	soa := testSOA(1)
	soa.Hdr.Class = dns.ClassINET
	if err := db1.CommitUpdate("example.org.", key, []dns.RR{soa}, dns.ClassINET); err != nil {
		t.Fatalf("CommitUpdate: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	defer db2.Close()

	store, keys, err := db2.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll after reopen: %v", err)
	}
	if _, ok := keys.Get("example.org."); !ok {
		t.Fatalf("expected the pinned key to survive closing and reopening the database")
	}
	z, ok := store.Get("example.org.")
	if !ok || z.SOA() == nil {
		t.Fatalf("expected the zone/SOA to survive closing and reopening the database")
	}
}

func TestDBCommitUpdateAppliesAllFourOpForms(t *testing.T) {
	db := openTestDB(t)
	key, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	soa := testSOA(1)
	soa.Hdr.Class = dns.ClassINET
	a1 := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	a1.Hdr.Class = dns.ClassINET
	a2 := testA("www.example.org.", net.IPv4(203, 0, 113, 11))
	a2.Hdr.Class = dns.ClassINET
	mx := &dns.MX{Hdr: dns.RR_Header{Name: "mail.example.org.", Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: 300}, Preference: 10, Mx: "mx.example.org."}

	if err := db.CommitUpdate("example.org.", key, []dns.RR{soa, a1, a2, mx}, dns.ClassINET); err != nil {
		t.Fatalf("CommitUpdate (onboard): %v", err)
	}

	// §2.5.4 delete one RR
	del := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	del.Hdr.Class = dns.ClassNONE
	if err := db.CommitUpdate("example.org.", nil, []dns.RR{del}, dns.ClassINET); err != nil {
		t.Fatalf("CommitUpdate (delete one RR): %v", err)
	}

	// §2.5.2 delete an RRset
	delRRset := &dns.MX{Hdr: dns.RR_Header{Name: "mail.example.org.", Rrtype: dns.TypeMX, Class: dns.ClassANY, Ttl: 0}}
	if err := db.CommitUpdate("example.org.", nil, []dns.RR{delRRset}, dns.ClassINET); err != nil {
		t.Fatalf("CommitUpdate (delete rrset): %v", err)
	}

	store, _, err := db.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	z, _ := store.Get("example.org.")
	got := z.Lookup("www.example.org.", dns.TypeA)
	if len(got) != 1 || !got[0].(*dns.A).A.Equal(net.IPv4(203, 0, 113, 11)) {
		t.Fatalf("expected only .11 to remain after deleting .10, got %+v", got)
	}
	if got := z.Lookup("mail.example.org.", dns.TypeMX); len(got) != 0 {
		t.Fatalf("expected the MX rrset to be gone, got %+v", got)
	}
}

func TestDBCommitUpdateRollsBackOnMalformedOp(t *testing.T) {
	db := openTestDB(t)
	key, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	soa := testSOA(1)
	soa.Hdr.Class = dns.ClassINET
	good := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	good.Hdr.Class = dns.ClassINET
	// A malformed op: neither Add (zone class), nor any recognized delete
	// shape -- CHAOS class with real rdata matches none of the four forms.
	bad := testA("bad.example.org.", net.IPv4(203, 0, 113, 99))
	bad.Hdr.Class = dns.ClassCHAOS

	err = db.CommitUpdate("example.org.", key, []dns.RR{soa, good, bad}, dns.ClassINET)
	if err == nil {
		t.Fatalf("expected CommitUpdate to fail on a malformed op")
	}

	store, keys, loadErr := db.LoadAll()
	if loadErr != nil {
		t.Fatalf("LoadAll: %v", loadErr)
	}
	if _, ok := keys.Get("example.org."); ok {
		t.Fatalf("expected nothing to be committed after a rolled-back transaction, but the key was pinned")
	}
	if _, ok := store.Get("example.org."); ok {
		t.Fatalf("expected nothing to be committed after a rolled-back transaction, but the zone exists")
	}
}
