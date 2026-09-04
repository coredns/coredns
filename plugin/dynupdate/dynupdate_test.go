package dynupdate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	coretest "github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/plugin/transfer"

	"github.com/miekg/dns"
)

const (
	testZone = "example.org."
	testKey  = "update-key.example.org."
)

func mustRR(t *testing.T, text string) dns.RR {
	t.Helper()
	rr, err := dns.NewRR(text)
	if err != nil {
		t.Fatalf("parsing RR %q: %v", text, err)
	}
	return rr
}

func withClass(t *testing.T, rr dns.RR, class uint16) dns.RR {
	t.Helper()
	rr = dns.Copy(rr)
	rr.Header().Class = class
	// PackRR fills RDLENGTH, which is significant for wire-format UPDATE
	// validation and makes the test record equivalent to a received RR.
	buf := make([]byte, dns.Len(rr)+1024)
	if _, err := dns.PackRR(rr, buf, 0, nil, false); err != nil {
		t.Fatalf("packing RR %q: %v", rr, err)
	}
	return rr
}

func emptyRR(name string, rrType, class uint16) dns.RR {
	return &dns.RFC3597{Hdr: dns.RR_Header{
		Name:   name,
		Rrtype: rrType,
		Class:  class,
	}}
}

func emptyA(name string, class uint16) dns.RR {
	return &dns.A{Hdr: dns.RR_Header{
		Name:   name,
		Rrtype: dns.TypeA,
		Class:  class,
	}}
}

func newTestDynUpdate(t *testing.T, extra ...string) *DynUpdate {
	t.Helper()
	texts := make([]string, 0, 4+len(extra))
	texts = append(texts,
		"example.org. 60 IN SOA ns.example.org. hostmaster.example.org. 10 60 60 60 60",
		"example.org. 60 IN NS ns.example.org.",
		"ns.example.org. 60 IN A 192.0.2.53",
		"www.example.org. 60 IN A 192.0.2.1",
	)
	texts = append(texts, extra...)
	records := make([]dns.RR, 0, len(texts))
	for _, text := range texts {
		records = append(records, mustRR(t, text))
	}
	d := &DynUpdate{
		Zone:    testZone,
		records: records,
		permissions: []permission{{
			key:      testKey,
			name:     allNames,
			allTypes: true,
		}},
	}
	var err error
	d.view, err = d.build(records)
	if err != nil {
		t.Fatalf("building test zone: %v", err)
	}
	return d
}

func serial(d *DynUpdate) uint32 {
	return soaAt(d.records, d.Zone).Serial
}

func hasRecord(d *DynUpdate, name string, rrType uint16, want string) bool {
	for _, rr := range d.rrset(name, rrType) {
		if want == "" || rr.String() == want {
			return true
		}
	}
	return false
}

func TestSerialArithmetic(t *testing.T) {
	tests := []struct {
		name    string
		a, b    uint32
		greater bool
	}{
		{name: "equal", a: 10, b: 10, greater: false},
		{name: "forward", a: 11, b: 10, greater: true},
		{name: "wrap", a: 1, b: ^uint32(0), greater: true},
		{name: "backward", a: 10, b: 11, greater: false},
		{name: "half-space", a: 1<<31 + 10, b: 10, greater: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serialGreater(tt.a, tt.b); got != tt.greater {
				t.Fatalf("serialGreater(%d, %d) = %v, want %v", tt.a, tt.b, got, tt.greater)
			}
		})
	}
}

func TestApplyAddAndSerial(t *testing.T) {
	d := newTestDynUpdate(t)
	add := mustRR(t, "new.example.org. 60 IN TXT \"first\"")
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{add}); got != dns.RcodeSuccess || err != nil {
		t.Fatalf("add returned rcode=%d err=%v", got, err)
	}
	if !hasRecord(d, "new.example.org.", dns.TypeTXT, "new.example.org.\t60\tIN\tTXT\t\"first\"") {
		t.Fatalf("added TXT record is missing")
	}
	if got := serial(d); got != 11 {
		t.Fatalf("serial after add = %d, want 11", got)
	}

	// A duplicate with the same TTL is a no-op and must not consume a serial.
	duplicate := mustRR(t, "new.example.org. 60 IN TXT \"first\"")
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{duplicate}); got != dns.RcodeSuccess || err != nil {
		t.Fatalf("duplicate returned rcode=%d err=%v", got, err)
	}
	if got := serial(d); got != 11 {
		t.Fatalf("serial after duplicate = %d, want 11", got)
	}

	// The same RDATA with a different TTL replaces the existing RR.
	ttlChange := mustRR(t, "new.example.org. 120 IN TXT \"first\"")
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{ttlChange}); got != dns.RcodeSuccess || err != nil {
		t.Fatalf("TTL change returned rcode=%d err=%v", got, err)
	}
	if got := serial(d); got != 12 {
		t.Fatalf("serial after TTL change = %d, want 12", got)
	}
	if rr := d.rrset("new.example.org.", dns.TypeTXT); len(rr) != 1 || rr[0].Header().Ttl != 120 {
		t.Fatalf("TTL change was not applied: %v", rr)
	}
}

func TestPrerequisites(t *testing.T) {
	d := newTestDynUpdate(t)
	existingA := emptyA("www.example.org.", dns.ClassANY)
	nonexistent := emptyA("missing.example.org.", dns.ClassANY)
	existingName := emptyRR("www.example.org.", dns.TypeANY, dns.ClassANY)
	missingName := emptyRR("missing.example.org.", dns.TypeANY, dns.ClassNONE)
	value := withClass(t, mustRR(t, "www.example.org. 0 IN A 192.0.2.1"), dns.ClassINET)
	value.Header().Ttl = 0
	wrongValue := withClass(t, mustRR(t, "www.example.org. 0 IN A 192.0.2.99"), dns.ClassINET)
	wrongValue.Header().Ttl = 0

	tests := []struct {
		name      string
		prereq    []dns.RR
		wantRcode int
	}{
		{name: "name in use", prereq: []dns.RR{existingName}, wantRcode: dns.RcodeSuccess},
		{name: "name not in use", prereq: []dns.RR{missingName}, wantRcode: dns.RcodeSuccess},
		{name: "rrset exists", prereq: []dns.RR{existingA}, wantRcode: dns.RcodeSuccess},
		{name: "rrset missing", prereq: []dns.RR{nonexistent}, wantRcode: dns.RcodeNXRrset},
		{name: "rrset does not exist", prereq: []dns.RR{emptyA("missing.example.org.", dns.ClassNONE)}, wantRcode: dns.RcodeSuccess},
		{name: "rrset exists unexpectedly", prereq: []dns.RR{emptyA("www.example.org.", dns.ClassNONE)}, wantRcode: dns.RcodeYXRrset},
		{name: "value dependent", prereq: []dns.RR{value}, wantRcode: dns.RcodeSuccess},
		{name: "value mismatch", prereq: []dns.RR{wrongValue}, wantRcode: dns.RcodeNXRrset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := serial(d)
			add := mustRR(t, "prereq-"+strings.ReplaceAll(tt.name, " ", "-")+".example.org. 60 IN TXT \"ok\"")
			got, err := d.applyUpdate(testKey, tt.prereq, []dns.RR{add})
			if err != nil || got != tt.wantRcode {
				t.Fatalf("got rcode=%d err=%v, want %d", got, err, tt.wantRcode)
			}
			if tt.wantRcode != dns.RcodeSuccess {
				if serial(d) != before {
					t.Fatalf("failed prerequisite changed serial from %d to %d", before, serial(d))
				}
				return
			}
			if !hasRecord(d, add.Header().Name, dns.TypeTXT, "") {
				t.Fatalf("successful prerequisite did not permit update")
			}
		})
	}
}

func TestUpdateOperationsAndApexProtection(t *testing.T) {
	d := newTestDynUpdate(t, "multi.example.org. 60 IN A 192.0.2.2", "multi.example.org. 60 IN TXT \"x\"")
	before := serial(d)

	deleteRRset := emptyA("multi.example.org.", dns.ClassANY)
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{deleteRRset}); got != dns.RcodeSuccess || err != nil {
		t.Fatalf("RRset delete returned rcode=%d err=%v", got, err)
	}
	if d.rrsetExists("multi.example.org.", dns.TypeA) || !d.rrsetExists("multi.example.org.", dns.TypeTXT) {
		t.Fatalf("RRset delete did not remove only the requested RRset")
	}
	if serial(d) != before+1 {
		t.Fatalf("serial after RRset delete = %d, want %d", serial(d), before+1)
	}
	deleteTXT := emptyRR("multi.example.org.", dns.TypeTXT, dns.ClassANY)
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{deleteTXT}); got != dns.RcodeSuccess || err != nil {
		t.Fatalf("second RRset delete returned rcode=%d err=%v", got, err)
	}
	if d.rrsetExists("multi.example.org.", dns.TypeTXT) {
		t.Fatalf("second RRset delete left records behind")
	}

	// Delete all data at the apex, while retaining SOA and NS as required by
	// RFC 2136.
	deleteAll := emptyRR(testZone, dns.TypeANY, dns.ClassANY)
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{deleteAll}); got != dns.RcodeSuccess || err != nil {
		t.Fatalf("apex delete returned rcode=%d err=%v", got, err)
	}
	if soaAt(d.records, testZone) == nil || !d.rrsetExists(testZone, dns.TypeNS) {
		t.Fatalf("apex delete removed SOA or NS")
	}

	// A last apex NS cannot be removed with an exact delete.
	lastNS := withClass(t, mustRR(t, "example.org. 0 IN NS ns.example.org."), dns.ClassNONE)
	lastNS.Header().Ttl = 0
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{lastNS}); got != dns.RcodeSuccess || err != nil {
		t.Fatalf("last NS delete returned rcode=%d err=%v", got, err)
	}
	if !d.rrsetExists(testZone, dns.TypeNS) {
		t.Fatalf("last apex NS was removed")
	}
}

func TestExplicitAnyPermission(t *testing.T) {
	d := newTestDynUpdate(t)
	d.permissions = []permission{{
		key:   testKey,
		name:  "www.example.org.",
		types: map[uint16]struct{}{dns.TypeANY: {}},
	}}

	deleteAll := emptyRR("www.example.org.", dns.TypeANY, dns.ClassANY)
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{deleteAll}); got != dns.RcodeSuccess || err != nil {
		t.Fatalf("explicit ANY delete returned rcode=%d err=%v", got, err)
	}
	if d.nameInUse("www.example.org.") {
		t.Fatal("explicit ANY permission did not remove the owner data")
	}

	add := mustRR(t, "www.example.org. 60 IN A 192.0.2.20")
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{add}); got != dns.RcodeRefused || err != nil {
		t.Fatalf("ordinary RR with ANY-only permission returned rcode=%d err=%v, want REFUSED", got, err)
	}
}

func TestCNAMERules(t *testing.T) {
	d := newTestDynUpdate(t)
	before := serial(d)
	// A CNAME cannot be added where ordinary data exists.
	cname := mustRR(t, "www.example.org. 60 IN CNAME target.example.org.")
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{cname}); got != dns.RcodeSuccess || err != nil {
		t.Fatalf("CNAME conflict returned rcode=%d err=%v", got, err)
	}
	if d.rrsetExists("www.example.org.", dns.TypeCNAME) || serial(d) != before {
		t.Fatalf("CNAME conflict changed the zone")
	}

	// Processed in order: deleting the A permits the later CNAME add.
	deleteA := withClass(t, mustRR(t, "www.example.org. 0 IN A 192.0.2.1"), dns.ClassNONE)
	deleteA.Header().Ttl = 0
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{deleteA, cname}); got != dns.RcodeSuccess || err != nil {
		t.Fatalf("ordered CNAME update returned rcode=%d err=%v", got, err)
	}
	if !d.rrsetExists("www.example.org.", dns.TypeCNAME) {
		t.Fatalf("ordered CNAME update did not add CNAME")
	}
	before = serial(d)
	duplicate := mustRR(t, "www.example.org. 60 IN CNAME target.example.org.")
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{duplicate}); got != dns.RcodeSuccess || err != nil {
		t.Fatalf("duplicate CNAME returned rcode=%d err=%v", got, err)
	}
	if serial(d) != before {
		t.Fatalf("duplicate CNAME consumed a serial")
	}
}

func TestValidationIsAtomic(t *testing.T) {
	d := newTestDynUpdate(t)
	before := serial(d)
	valid := mustRR(t, "new.example.org. 60 IN A 192.0.2.10")
	outOfZone := mustRR(t, "outside.test. 60 IN A 192.0.2.11")
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{valid, outOfZone}); got != dns.RcodeNotZone || err != nil {
		t.Fatalf("out-of-zone transaction returned rcode=%d err=%v", got, err)
	}
	if d.rrsetExists("new.example.org.", dns.TypeA) || serial(d) != before {
		t.Fatalf("failed prescan partially changed the zone")
	}

	malformed := emptyA("new.example.org.", dns.ClassANY)
	malformed.Header().Ttl = 1
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{malformed}); got != dns.RcodeFormatError || err != nil {
		t.Fatalf("malformed delete returned rcode=%d err=%v", got, err)
	}

	d.permissions = []permission{{key: testKey, name: "allowed.example.org.", types: map[uint16]struct{}{dns.TypeA: {}}}}
	unauthorized := mustRR(t, "blocked.example.org. 60 IN A 192.0.2.12")
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{unauthorized}); got != dns.RcodeRefused || err != nil {
		t.Fatalf("unauthorized update returned rcode=%d err=%v", got, err)
	}
}

func TestPrerequisitesPrecedeUpdatePrescan(t *testing.T) {
	d := newTestDynUpdate(t)
	missingRRset := emptyA("missing.example.org.", dns.ClassANY)
	outOfZone := mustRR(t, "outside.test. 60 IN A 192.0.2.13")

	got, err := d.applyUpdate(testKey, []dns.RR{missingRRset}, []dns.RR{outOfZone})
	if err != nil || got != dns.RcodeNXRrset {
		t.Fatalf("transaction returned rcode=%d err=%v, want NXRRSET", got, err)
	}
	if d.rrsetExists("outside.test.", dns.TypeA) {
		t.Fatalf("failed prerequisite allowed an out-of-zone update")
	}
}

func TestSerialWrap(t *testing.T) {
	d := newTestDynUpdate(t)
	d.records[0].(*dns.SOA).Serial = ^uint32(0)
	newRecord := mustRR(t, "wrap.example.org. 60 IN TXT \"x\"")
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{newRecord}); got != dns.RcodeSuccess || err != nil {
		t.Fatalf("wrap update returned rcode=%d err=%v", got, err)
	}
	if got := serial(d); got != 1 {
		t.Fatalf("wrapped serial = %d, want 1", got)
	}
}

func TestServeUpdateRequiresValidatedTSIG(t *testing.T) {
	d := newTestDynUpdate(t)
	r := new(dns.Msg)
	r.SetQuestion(testZone, dns.TypeSOA)
	r.Opcode = dns.OpcodeUpdate
	w := dnstest.NewRecorder(&coretest.ResponseWriter{})
	code, err := d.ServeDNS(context.Background(), w, r)
	if err != nil || code != dns.RcodeSuccess {
		t.Fatalf("ServeDNS returned code=%d err=%v", code, err)
	}
	if w.Msg == nil || w.Msg.Rcode != dns.RcodeRefused {
		t.Fatalf("unsigned update response = %#v, want REFUSED", w.Msg)
	}
	if w.Msg.Opcode != dns.OpcodeUpdate {
		t.Fatalf("response opcode = %d, want UPDATE", w.Msg.Opcode)
	}
}

func TestServeUpdateDelegatesOtherZones(t *testing.T) {
	d := newTestDynUpdate(t)
	called := false
	d.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		called = true
		m := new(dns.Msg).SetReply(r)
		if err := w.WriteMsg(m); err != nil {
			return dns.RcodeServerFailure, err
		}
		return dns.RcodeSuccess, nil
	})

	r := new(dns.Msg)
	r.SetQuestion("other.example.", dns.TypeSOA)
	r.Opcode = dns.OpcodeUpdate
	w := dnstest.NewRecorder(&coretest.ResponseWriter{})
	code, err := d.ServeDNS(context.Background(), w, r)
	if err != nil || code != dns.RcodeSuccess {
		t.Fatalf("delegated UPDATE returned code=%d err=%v", code, err)
	}
	if !called {
		t.Fatal("UPDATE for another zone did not reach the next handler")
	}
	if w.Msg == nil || w.Msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("delegated response = %#v, want NOERROR", w.Msg)
	}
}

func TestTransferUsesCurrentSnapshot(t *testing.T) {
	d := newTestDynUpdate(t)
	added := mustRR(t, "transfer.example.org. 60 IN TXT \"dynamic\"")
	if got, err := d.applyUpdate(testKey, nil, []dns.RR{added}); got != dns.RcodeSuccess || err != nil {
		t.Fatalf("dynamic update returned rcode=%d err=%v", got, err)
	}
	ch, err := d.Transfer(testZone, 0)
	if err != nil {
		t.Fatalf("Transfer returned error: %v", err)
	}
	var records []dns.RR
	for batch := range ch {
		records = append(records, batch...)
	}
	if len(records) < 2 || records[0].Header().Rrtype != dns.TypeSOA || records[len(records)-1].Header().Rrtype != dns.TypeSOA {
		t.Fatalf("unexpected transfer framing: %v", records)
	}
	found := false
	for _, rr := range records {
		if sameRR(rr, added) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("transfer did not include dynamic record")
	}

	ch, err = d.Transfer("EXAMPLE.ORG", 0)
	if err != nil || ch == nil {
		t.Fatalf("case-insensitive transfer returned channel=%v err=%v", ch, err)
	}
	for range ch {
	}
	if _, err := d.Transfer("child.example.org.", 0); err != transfer.ErrNotAuthoritative {
		t.Fatalf("subdomain transfer error = %v, want ErrNotAuthoritative", err)
	}
}

func TestExternalCNAMEUsesInitializedUpstream(t *testing.T) {
	d := newTestDynUpdate(t, "alias.example.org. 60 IN CNAME external.test.")
	r := new(dns.Msg)
	r.SetQuestion("alias.example.org.", dns.TypeA)
	w := dnstest.NewRecorder(&coretest.ResponseWriter{})

	code, err := d.ServeDNS(context.Background(), w, r)
	if err != nil {
		t.Fatalf("ServeDNS returned error: %v", err)
	}
	if code != dns.RcodeSuccess || w.Msg == nil || w.Msg.Rcode != dns.RcodeServerFailure {
		t.Fatalf("external CNAME response = code %d, message %#v; want written SERVFAIL", code, w.Msg)
	}
}

func TestConcurrentUpdates(t *testing.T) {
	d := newTestDynUpdate(t)
	const updates = 16
	updateRecords := make([]dns.RR, updates)
	for i := range updates {
		updateRecords[i] = mustRR(t, fmt.Sprintf("node-%d.example.org. 60 IN A 192.0.2.%d", i, i+20))
	}
	var wg sync.WaitGroup
	for i := range updates {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if got, err := d.applyUpdate(testKey, nil, []dns.RR{updateRecords[i]}); got != dns.RcodeSuccess || err != nil {
				t.Errorf("update %d returned rcode=%d err=%v", i, got, err)
			}
		}(i)
	}
	wg.Wait()
	if got := serial(d); got != 10+updates {
		t.Fatalf("serial after concurrent updates = %d, want %d", got, 10+updates)
	}
	for i := range updates {
		if !d.rrsetExists(fmt.Sprintf("node-%d.example.org.", i), dns.TypeA) {
			t.Errorf("missing concurrent update %d", i)
		}
	}
}

func TestParse(t *testing.T) {
	dir := t.TempDir()
	zonePath := filepath.Join(dir, "db.example.org")
	zoneText := strings.Join([]string{
		"$ORIGIN example.org.",
		"@ 60 IN SOA ns.example.org. hostmaster.example.org. 10 60 60 60 60",
		"@ 60 IN NS ns.example.org.",
		"ns 60 IN A 192.0.2.53",
	}, "\n")
	if err := os.WriteFile(zonePath, []byte(zoneText), 0600); err != nil {
		t.Fatalf("writing test zone: %v", err)
	}

	c := caddy.NewTestController("dns", `dynupdate {
		file db.example.org
		allow update-key.example.org. * A TXT
	}`)
	c.ServerBlockKeys = []string{testZone}
	dnsserver.GetConfig(c).Root = dir
	d, err := parse(c)
	if err != nil {
		t.Fatalf("parse returned error: %v", err)
	}
	if d.Zone != testZone || len(d.records) != 3 {
		t.Fatalf("parsed zone = %q with %d records", d.Zone, len(d.records))
	}
	if !d.allows(testKey, "new.example.org.", dns.TypeTXT) || d.allows(testKey, "new.example.org.", dns.TypeAAAA) {
		t.Fatalf("parsed allow rule has unexpected permissions")
	}

	explicit := caddy.NewTestController("dns", `dynupdate example.org. {
		file db.example.org
		allow update-key.example.org. * A
	}`)
	explicit.ServerBlockKeys = []string{testZone, "other.example."}
	dnsserver.GetConfig(explicit).Root = dir
	explicitDynUpdate, err := parse(explicit)
	if err != nil {
		t.Fatalf("explicit zone in a multi-zone server block returned err=%v", err)
	}
	if explicitDynUpdate.Zone != testZone {
		t.Fatalf("explicit zone in a multi-zone server block returned zone=%q", explicitDynUpdate.Zone)
	}
}

func TestParseRejectsUnsafeConfiguration(t *testing.T) {
	dir := t.TempDir()
	zoneText := strings.Join([]string{
		"$ORIGIN example.org.",
		"@ 60 IN SOA ns.example.org. hostmaster.example.org. 10 60 60 60 60",
		"@ 60 IN NS ns.example.org.",
		"ns 60 IN A 192.0.2.53",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "db.example.org"), []byte(zoneText), 0600); err != nil {
		t.Fatalf("writing test zone: %v", err)
	}

	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing file",
			body: `dynupdate {
				allow update-key.example.org. * A
			}`,
		},
		{
			name: "missing allow",
			body: `dynupdate {
				file db.example.org
			}`,
		},
		{
			name: "unknown policy type",
			body: `dynupdate {
				file db.example.org
				allow update-key.example.org. * TYPE65280
			}`,
		},
		{
			name: "wildcard type mixed with named type",
			body: `dynupdate {
				file db.example.org
				allow update-key.example.org. * * TXT
			}`,
		},
		{
			name: "invalid key wildcard",
			body: `dynupdate {
				file db.example.org
				allow * * TXT
			}`,
		},
		{
			name: "owner outside zone",
			body: `dynupdate {
				file db.example.org
				allow update-key.example.org. outside.test. TXT
			}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tt.body)
			c.ServerBlockKeys = []string{testZone}
			dnsserver.GetConfig(c).Root = dir
			if _, err := parse(c); err == nil {
				t.Fatalf("parse accepted unsafe configuration")
			}
		})
	}

	validAny := caddy.NewTestController("dns", `dynupdate {
		file db.example.org
		allow update-key.example.org. host.example.org. ANY
	}`)
	validAny.ServerBlockKeys = []string{testZone}
	dnsserver.GetConfig(validAny).Root = dir
	d, err := parse(validAny)
	if err != nil {
		t.Fatalf("parse rejected an explicit ANY update permission: %v", err)
	}
	if !d.allows(testKey, "host.example.org.", dns.TypeANY) {
		t.Fatal("explicit ANY update permission was not recorded")
	}

	c := caddy.NewTestController("dns", `dynupdate {
		file db.example.org
		allow update-key.example.org. * A
	}`)
	c.ServerBlockKeys = []string{testZone, "other.example."}
	dnsserver.GetConfig(c).Root = dir
	if _, err := parse(c); err == nil {
		t.Fatalf("parse accepted a server block with multiple implicit zones")
	}
}

func TestReadZoneRejectsInvalidZoneData(t *testing.T) {
	tests := []struct {
		name string
		zone string
	}{
		{
			name: "missing SOA",
			zone: "$ORIGIN example.org.\n@ 60 IN NS ns.example.org.\n",
		},
		{
			name: "duplicate SOA",
			zone: "$ORIGIN example.org.\n@ 60 IN SOA ns.example.org. hostmaster.example.org. 10 60 60 60 60\n@ 60 IN SOA ns.example.org. hostmaster.example.org. 11 60 60 60 60\n",
		},
		{
			name: "zero serial",
			zone: "$ORIGIN example.org.\n@ 60 IN SOA ns.example.org. hostmaster.example.org. 0 60 60 60 60\n",
		},
		{
			name: "CNAME data conflict",
			zone: "$ORIGIN example.org.\n@ 60 IN SOA ns.example.org. hostmaster.example.org. 10 60 60 60 60\nalias 60 IN CNAME target.example.org.\nalias 60 IN A 192.0.2.20\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "db.example.org")
			if err := os.WriteFile(path, []byte(tt.zone), 0600); err != nil {
				t.Fatalf("writing zone: %v", err)
			}
			if _, err := readZone(path, testZone); err == nil {
				t.Fatalf("readZone accepted %s", tt.name)
			}
		})
	}
}

func TestUnsupportedMetadataIsRejected(t *testing.T) {
	unsupported := []uint16{
		dns.TypeSIG, dns.TypeKEY, dns.TypeNXT,
		dns.TypeDS, dns.TypeRRSIG, dns.TypeNSEC, dns.TypeDNSKEY,
		dns.TypeNSEC3, dns.TypeNSEC3PARAM,
		dns.TypeTALINK, dns.TypeCDS, dns.TypeCDNSKEY,
		dns.TypeZONEMD, dns.TypeTA, dns.TypeDLV,
	}
	for _, rrType := range unsupported {
		t.Run(dns.TypeToString[rrType], func(t *testing.T) {
			soa := mustRR(t, "example.org. 60 IN SOA ns.example.org. hostmaster.example.org. 10 60 60 60 60")
			rr := dns.TypeToRR[rrType]()
			rr.Header().Name = "record.example.org."
			rr.Header().Rrtype = rrType
			rr.Header().Class = dns.ClassINET
			if err := validateRecords([]dns.RR{soa, rr}, testZone); err == nil {
				t.Fatalf("validateRecords accepted %s", dns.TypeToString[rrType])
			}

			d := newTestDynUpdate(t)
			before := serial(d)
			if got, err := d.applyUpdate(testKey, nil, []dns.RR{rr}); got != dns.RcodeNotImplemented || err != nil {
				t.Fatalf("update returned rcode=%d err=%v, want NOTIMP", got, err)
			}
			if serial(d) != before {
				t.Fatalf("rejected metadata update changed serial")
			}
		})
	}
}
