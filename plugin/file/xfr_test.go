package file

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/plugin/transfer"

	"github.com/miekg/dns"
)

func ExampleZone_All() {
	zone, err := Parse(strings.NewReader(dbMiekNL), testzone, "stdin", 0)
	if err != nil {
		return
	}
	records := zone.All()
	for _, r := range records {
		fmt.Printf("%+v\n", r)
	}
	// Output
	// xfr_test.go:15: miek.nl.	1800	IN	SOA	linode.atoom.net. miek.miek.nl. 1282630057 14400 3600 604800 14400
	// xfr_test.go:15: www.miek.nl.	1800	IN	CNAME	a.miek.nl.
	// xfr_test.go:15: miek.nl.	1800	IN	NS	linode.atoom.net.
	// xfr_test.go:15: miek.nl.	1800	IN	NS	ns-ext.nlnetlabs.nl.
	// xfr_test.go:15: miek.nl.	1800	IN	NS	omval.tednet.nl.
	// xfr_test.go:15: miek.nl.	1800	IN	NS	ext.ns.whyscream.net.
	// xfr_test.go:15: miek.nl.	1800	IN	MX	1 aspmx.l.google.com.
	// xfr_test.go:15: miek.nl.	1800	IN	MX	5 alt1.aspmx.l.google.com.
	// xfr_test.go:15: miek.nl.	1800	IN	MX	5 alt2.aspmx.l.google.com.
	// xfr_test.go:15: miek.nl.	1800	IN	MX	10 aspmx2.googlemail.com.
	// xfr_test.go:15: miek.nl.	1800	IN	MX	10 aspmx3.googlemail.com.
	// xfr_test.go:15: miek.nl.	1800	IN	A	139.162.196.78
	// xfr_test.go:15: miek.nl.	1800	IN	AAAA	2a01:7e00::f03c:91ff:fef1:6735
	// xfr_test.go:15: archive.miek.nl.	1800	IN	CNAME	a.miek.nl.
	// xfr_test.go:15: a.miek.nl.	1800	IN	A	139.162.196.78
	// xfr_test.go:15: a.miek.nl.	1800	IN	AAAA	2a01:7e00::f03c:91ff:fef1:6735
}

func TestAllNewZone(t *testing.T) {
	zone := NewZone("example.org.", "stdin")
	records := zone.All()
	if len(records) != 0 {
		t.Errorf("Expected %d records in empty zone, got %d", 0, len(records))
	}
}

func TestAXFRWithOutTransferPlugin(t *testing.T) {
	zone, err := Parse(strings.NewReader(dbMiekNL), testzone, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{testzone: zone}, Names: []string{testzone}}}
	ctx := context.TODO()

	m := new(dns.Msg)
	m.SetQuestion("miek.nl.", dns.TypeAXFR)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	code, err := fm.ServeDNS(ctx, rec, m)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
		return
	}
	if code != dns.RcodeRefused {
		t.Errorf("Expecting REFUSED, got %d", code)
	}
}

func TestTransferRequiresExactZone(t *testing.T) {
	zone, err := Parse(strings.NewReader(dbMiekNL), testzone, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	fm := File{Zones: Zones{Z: map[string]*Zone{testzone: zone}, Names: []string{testzone}}}
	if _, err := fm.Transfer("www."+testzone, 0); err != transfer.ErrNotAuthoritative {
		t.Fatalf("expected ErrNotAuthoritative for subdomain transfer, got %v", err)
	}
}

func TestZoneInsertSuppressesDuplicateApexRecords(t *testing.T) {
	zone, _ := zoneWithDuplicateApexRecords(t)

	if got := len(zone.NS); got != 2 {
		t.Errorf("Expected duplicate apex NS records to be suppressed, got %d records", got)
	}
	if got := len(zone.SIGSOA); got != 2 {
		t.Errorf("Expected duplicate apex RRSIG(SOA) records to be suppressed, got %d records", got)
	}
	if got := len(zone.SIGNS); got != 2 {
		t.Errorf("Expected duplicate apex RRSIG(NS) records to be suppressed, got %d records", got)
	}
}

func TestZoneTransferSuppressesDuplicateApexRecords(t *testing.T) {
	zone, want := zoneWithDuplicateApexRecords(t)

	records, err := zone.Transfer(0)
	if err != nil {
		t.Fatalf("Expected no error transferring zone, got %v", err)
	}

	var transferred []dns.RR
	for batch := range records {
		transferred = append(transferred, batch...)
	}

	for _, rr := range want {
		if got := countDuplicateRR(transferred, rr); got != 1 {
			t.Errorf("Expected AXFR to emit %q once, got %d copies", rr, got)
		}
	}
}

func zoneWithDuplicateApexRecords(t *testing.T) (*Zone, []dns.RR) {
	t.Helper()

	zone := NewZone("example.org.", "stdin")
	records := []struct {
		text      string
		duplicate bool
	}{
		{"example.org. 3600 IN SOA ns.example.org. hostmaster.example.org. 1 3600 1800 604800 300", false},
		{"example.org. 3600 IN NS ns.example.org.", false},
		{"example.org. 3600 IN NS ns.example.org.", true},
		{"example.org. 3600 IN NS ns2.example.org.", false},
		{"example.org. 3600 IN RRSIG SOA 8 2 3600 20300101000000 20260101000000 12345 example.org. AQID", false},
		{"example.org. 3600 IN RRSIG SOA 8 2 3600 20300101000000 20260101000000 12345 example.org. AQID", true},
		{"example.org. 3600 IN RRSIG SOA 8 2 3600 20300101000000 20260101000000 12345 example.org. BAUG", false},
		{"example.org. 3600 IN RRSIG NS 8 2 3600 20300101000000 20260101000000 12345 example.org. AQID", false},
		{"example.org. 3600 IN RRSIG NS 8 2 3600 20300101000000 20260101000000 12345 example.org. AQID", true},
		{"example.org. 3600 IN RRSIG NS 8 2 3600 20300101000000 20260101000000 12345 example.org. BAUG", false},
	}

	var want []dns.RR
	for _, record := range records {
		rr, err := dns.NewRR(record.text)
		if err != nil {
			t.Fatalf("Failed to parse test record %q: %v", record.text, err)
		}
		if err := zone.Insert(rr); err != nil {
			t.Fatalf("Failed to insert test record %q: %v", record.text, err)
		}
		if rr.Header().Rrtype != dns.TypeSOA && !record.duplicate {
			want = append(want, rr)
		}
	}

	return zone, want
}

func countDuplicateRR(records []dns.RR, want dns.RR) int {
	count := 0
	for _, rr := range records {
		if dns.IsDuplicate(rr, want) {
			count++
		}
	}
	return count
}
