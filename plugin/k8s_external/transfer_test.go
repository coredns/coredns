package external

import (
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/kubernetes"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/plugin/transfer"

	"github.com/miekg/dns"
)

func TestImplementsTransferer(t *testing.T) {
	var e plugin.Handler
	e = &External{}
	_, ok := e.(transfer.Transferer)
	if !ok {
		t.Error("Transferer not implemented")
	}
}

func TestTransferAXFR(t *testing.T) {
	k := kubernetes.New([]string{"cluster.local."})
	k.Namespaces = map[string]struct{}{"testns": {}}
	k.APIConn = &external{}

	e := New()
	e.headless = true
	e.Zones = []string{"example.com."}
	e.externalFunc = k.External
	e.externalAddrFunc = externalAddress  // internal test function
	e.externalSerialFunc = externalSerial // internal test function
	e.externalServicesFunc = k.ExternalServices

	ch, err := e.Transfer("example.com.", 0)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	var records []dns.RR
	for rrs := range ch {
		records = append(records, rrs...)
	}

	expect := []dns.RR{}
	for _, tc := range append(tests, testsApex...) {
		if tc.Rcode != dns.RcodeSuccess {
			continue
		}

		for _, ans := range tc.Answer {
			// Exclude wildcard test cases
			if strings.Contains(ans.Header().Name, "*") {
				continue
			}

			// Exclude TXT records
			if ans.Header().Rrtype == dns.TypeTXT {
				continue
			}

			// Exclude PTR records
			if ans.Header().Rrtype == dns.TypePTR {
				continue
			}

			expect = append(expect, ans)
		}
	}

	diff := difference(expect, records)
	if len(diff) != 0 {
		t.Errorf("Got back %d records that do not exist in test cases, should be 0:", len(diff))
		for _, rec := range diff {
			t.Errorf("%+v", rec)
		}
	}

	diff = difference(records, expect)
	if len(diff) != 0 {
		t.Errorf("Result is missing %d records, should be 0:", len(diff))
		for _, rec := range diff {
			t.Errorf("%+v", rec)
		}
	}
}

func TestTransferIXFR(t *testing.T) {
	k := kubernetes.New([]string{"cluster.local."})
	k.Namespaces = map[string]struct{}{"testns": {}}
	k.APIConn = &external{}

	e := New()
	e.Zones = []string{"example.com."}
	e.headless = true
	e.externalFunc = k.External
	e.externalAddrFunc = externalAddress  // internal test function
	e.externalSerialFunc = externalSerial // internal test function
	e.externalServicesFunc = k.ExternalServices

	ch, err := e.Transfer("example.com.", externalSerial("example.com."))

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	var records []dns.RR
	for rrs := range ch {
		records = append(records, rrs...)
	}

	expect := []dns.RR{
		test.SOA("example.com.	5	IN	SOA	ns1.dns.example.com. hostmaster.dns.example.com. 1499347823 7200 1800 86400 5"),
	}

	diff := difference(expect, records)
	if len(diff) != 0 {
		t.Errorf("Got back %d records that do not exist in test cases, should be 0:", len(diff))
		for _, rec := range diff {
			t.Errorf("%+v", rec)
		}
	}

	diff = difference(records, expect)
	if len(diff) != 0 {
		t.Errorf("Result is missing %d records, should be 0:", len(diff))
		for _, rec := range diff {
			t.Errorf("%+v", rec)
		}
	}
}

// difference shows what we're missing when comparing two RR slices
func difference(testRRs []dns.RR, gotRRs []dns.RR) []dns.RR {
	expectedRRs := map[string]struct{}{}
	for _, rr := range testRRs {
		expectedRRs[rr.String()] = struct{}{}
	}

	foundRRs := []dns.RR{}
	for _, rr := range gotRRs {
		if _, ok := expectedRRs[rr.String()]; !ok {
			foundRRs = append(foundRRs, rr)
		}
	}
	return foundRRs
}
