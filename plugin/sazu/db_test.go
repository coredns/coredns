package sazu

import (
	"net"
	"path/filepath"
	"testing"
	"time"

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

	if err := db.CommitUpdate("example.org.", key, ops, dns.ClassINET, nil); err != nil {
		t.Fatalf("CommitUpdate: %v", err)
	}

	store, keys, _, err := db.LoadAll()
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

// TestDBCommitUpdatePurgesStaleNSECOnNextUpdate proves CommitUpdate's SQL
// mirrors ZoneData.PurgeNSEC exactly: an NSEC (and its RRSIG) persisted by
// one update must not survive a later update that doesn't include one,
// even across a full reload from disk -- otherwise a restarted server
// would resurrect a stale chain that live, in-memory traffic already
// correctly discarded.
func TestDBCommitUpdatePurgesStaleNSECOnNextUpdate(t *testing.T) {
	db := openTestDB(t)

	key, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	dnskeyRR := &dns.DNSKEY{Hdr: dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
		Flags: key.Flags, Protocol: key.Protocol, Algorithm: key.Algorithm, PublicKey: key.PublicKey}
	nsec := &dns.NSEC{Hdr: dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeNSEC, Class: dns.ClassINET, Ttl: 3600},
		NextDomain: "example.org.", TypeBitMap: []uint16{dns.TypeNSEC}}
	sig := &dns.RRSIG{Hdr: dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 3600},
		TypeCovered: dns.TypeNSEC, Algorithm: 15, KeyTag: 1, SignerName: "example.org."}
	firstOps := []dns.RR{dnskeyRR, testSOA(1), nsec, sig}
	for _, rr := range firstOps {
		rr.Header().Class = dns.ClassINET
	}
	if err := db.CommitUpdate("example.org.", key, firstOps, dns.ClassINET, nil); err != nil {
		t.Fatalf("first CommitUpdate: %v", err)
	}

	secondOps := []dns.RR{testA("www.example.org.", net.IPv4(203, 0, 113, 10))}
	secondOps[0].Header().Class = dns.ClassINET
	if err := db.CommitUpdate("example.org.", nil, secondOps, dns.ClassINET, nil); err != nil {
		t.Fatalf("second CommitUpdate: %v", err)
	}

	store, _, _, err := db.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	z, ok := store.Get("example.org.")
	if !ok {
		t.Fatalf("expected the zone to exist after loading")
	}
	if got := z.Lookup("example.org.", dns.TypeNSEC); len(got) != 0 {
		t.Fatalf("expected the stale NSEC to be purged, got %+v", got)
	}
	if got := z.LookupRRSIG("example.org.", dns.TypeNSEC); len(got) != 0 {
		t.Fatalf("expected the stale NSEC's RRSIG to be purged, got %+v", got)
	}
	if got := z.Lookup("www.example.org.", dns.TypeA); len(got) != 1 {
		t.Fatalf("expected the second update's own content to survive, got %+v", got)
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
	if err := db1.CommitUpdate("example.org.", key, []dns.RR{soa}, dns.ClassINET, nil); err != nil {
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

	store, keys, _, err := db2.LoadAll()
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

	if err := db.CommitUpdate("example.org.", key, []dns.RR{soa, a1, a2, mx}, dns.ClassINET, nil); err != nil {
		t.Fatalf("CommitUpdate (onboard): %v", err)
	}

	// §2.5.4 delete one RR
	del := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	del.Hdr.Class = dns.ClassNONE
	if err := db.CommitUpdate("example.org.", nil, []dns.RR{del}, dns.ClassINET, nil); err != nil {
		t.Fatalf("CommitUpdate (delete one RR): %v", err)
	}

	// §2.5.2 delete an RRset
	delRRset := &dns.MX{Hdr: dns.RR_Header{Name: "mail.example.org.", Rrtype: dns.TypeMX, Class: dns.ClassANY, Ttl: 0}}
	if err := db.CommitUpdate("example.org.", nil, []dns.RR{delRRset}, dns.ClassINET, nil); err != nil {
		t.Fatalf("CommitUpdate (delete rrset): %v", err)
	}

	store, _, _, err := db.LoadAll()
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

	err = db.CommitUpdate("example.org.", key, []dns.RR{soa, good, bad}, dns.ClassINET, nil)
	if err == nil {
		t.Fatalf("expected CommitUpdate to fail on a malformed op")
	}

	store, keys, _, loadErr := db.LoadAll()
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

// TestDBCommitUpdatePersistsAndClearsContact proves §10.6's registration
// record survives a restart (LoadAll rehydrates ContactRegistry, not just
// Store/KeyRegistry) and that a later clearing update actually removes the
// row rather than leaving stale contact data behind.
func TestDBCommitUpdatePersistsAndClearsContact(t *testing.T) {
	db := openTestDB(t)
	key, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	soa := testSOA(1)
	soa.Hdr.Class = dns.ClassINET

	if err := db.CommitUpdate("example.org.", key, []dns.RR{soa}, dns.ClassINET,
		&ContactUpdate{Addresses: []string{"mailto:ops@example.org", "https://hooks.example.org/sazu"}}); err != nil {
		t.Fatalf("CommitUpdate with contact: %v", err)
	}

	_, _, contacts, err := db.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	got, ok := contacts.Get("example.org.")
	if !ok || len(got) != 2 || got[0] != "mailto:ops@example.org" || got[1] != "https://hooks.example.org/sazu" {
		t.Fatalf("expected the registered contact to survive a round trip, got %+v ok=%v", got, ok)
	}

	if err := db.CommitUpdate("example.org.", nil, nil, dns.ClassINET, &ContactUpdate{}); err != nil {
		t.Fatalf("CommitUpdate clearing contact: %v", err)
	}
	_, _, contacts, err = db.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll after clearing: %v", err)
	}
	if _, ok := contacts.Get("example.org."); ok {
		t.Fatalf("expected the cleared contact to not survive a round trip")
	}
}

// TestDBRecordTransactionAndRecentTransactions proves §12's audit trail
// persistence: entries survive, come back newest first, and a zone with
// no entries at all (rather than an error) just gets an empty result --
// exactly what a never-onboarded zone's first, rejected attempt would
// look like before any later ones exist.
func TestDBRecordTransactionAndRecentTransactions(t *testing.T) {
	db := openTestDB(t)

	base := time.Now()
	entries := []AuditEntry{
		{ID: "tx-1", Zone: "example.org.", RemoteAddr: "203.0.113.1:5353", Rcode: "REFUSED", Status: "ERR_NO_DS_PUBLISHED", At: base},
		{ID: "tx-2", Zone: "example.org.", RemoteAddr: "203.0.113.1:5353", Rcode: "NOERROR", Status: "", At: base.Add(time.Minute)},
		{ID: "tx-3", Zone: "other.example.", RemoteAddr: "203.0.113.2:5353", Rcode: "NOERROR", Status: "", At: base.Add(2 * time.Minute)},
	}
	for _, e := range entries {
		if err := db.RecordTransaction(e); err != nil {
			t.Fatalf("RecordTransaction(%s): %v", e.ID, err)
		}
	}

	got, err := db.RecentTransactions("example.org.", 10)
	if err != nil {
		t.Fatalf("RecentTransactions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries for example.org., got %d: %+v", len(got), got)
	}
	if got[0].ID != "tx-2" || got[1].ID != "tx-1" {
		t.Fatalf("expected newest-first order (tx-2, tx-1), got (%s, %s)", got[0].ID, got[1].ID)
	}
	if got[1].Status != "ERR_NO_DS_PUBLISHED" {
		t.Fatalf("expected the rejected attempt's status to survive, got %q", got[1].Status)
	}

	if got, err := db.RecentTransactions("never-touched.example.", 10); err != nil || len(got) != 0 {
		t.Fatalf("expected no entries (not an error) for an untouched zone, got %+v err=%v", got, err)
	}

	if limited, err := db.RecentTransactions("example.org.", 1); err != nil || len(limited) != 1 || limited[0].ID != "tx-2" {
		t.Fatalf("expected limit to cap results to the single newest entry, got %+v err=%v", limited, err)
	}
}
