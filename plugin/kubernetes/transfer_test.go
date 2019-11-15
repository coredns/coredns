package kubernetes

import (
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/miekg/dns"
)

func TestImplementsTransferer(t *testing.T){
	var e plugin.Handler
	e = &Kubernetes{}
	_, ok := e.(transfer.Transferer)
	if !ok {
		t.Error("Transferer not implemented")
	}
}

func TestKubernetesAXFR(t *testing.T) {
	k := New([]string{"cluster.local."})
	k.APIConn = &APIConnServeTest{}
	k.Namespaces = map[string]struct{}{"testns": {}}

	dnsmsg := &dns.Msg{}
	dnsmsg.SetAxfr(k.Zones[0])

	ch, err := k.Transfer(k.Zones[0], 0)
	if err != nil {
		t.Error(err)
	}
	validateAXFR(t, ch)
}

func TestKubernetesIXFRFallback(t *testing.T) {
	k := New([]string{"cluster.local."})
	k.APIConn = &APIConnServeTest{}
	k.Namespaces = map[string]struct{}{"testns": {}}

	dnsmsg := &dns.Msg{}
	dnsmsg.SetAxfr(k.Zones[0])

	ch, err := k.Transfer(k.Zones[0], 1)
	if err != nil {
		t.Error(err)
	}
	validateAXFR(t, ch)
}

func TestKubernetesIXFRCurrent(t *testing.T) {
	k := New([]string{"cluster.local."})
	k.APIConn = &APIConnServeTest{}
	k.Namespaces = map[string]struct{}{"testns": {}}

	dnsmsg := &dns.Msg{}
	dnsmsg.SetAxfr(k.Zones[0])

	ch, err := k.Transfer(k.Zones[0], 2)
	if err != nil {
		t.Error(err)
	}

	var gotRRs []dns.RR
	for rrs := range ch {
		gotRRs = append(gotRRs, rrs...)
	}

	// ensure only one record is returned
	if len(gotRRs) > 1 {
		t.Errorf("Expected only one answer, got %d", len(gotRRs))
	}

	// Ensure first record is a SOA
	if gotRRs[0].Header().Rrtype != dns.TypeSOA {
		t.Error("Invalid transfer response, does not start with SOA record")
	}
}

func validateAXFR(t *testing.T, ch <-chan []dns.RR) {
	var gotRRs []dns.RR
	for rrs := range ch {
		gotRRs = append(gotRRs, rrs...)
	}

	// Ensure first record is a SOA
	if gotRRs[0].Header().Rrtype != dns.TypeSOA {
		t.Error("Invalid transfer response, does not start with SOA record")
	}

	// Compare remaining records after SOA
	gotRRs = gotRRs[1:]

	// Build the set of expected records from the test cases defined in dnsTestCases, from handler_test.go
	testRRs := []dns.RR{}
	for _, tc := range dnsTestCases {
		// exclude negative answer tests, wildcard search tests, and TXT records
		if tc.Rcode != dns.RcodeSuccess {
			continue
		}
		for _, ans := range tc.Answer {
			if strings.Contains(ans.Header().Name, "*") {
				continue
			}
			if ans.Header().Rrtype == dns.TypeTXT {
				continue
			}
			testRRs = append(testRRs, ans)
		}
	}

	diff := difference(testRRs, gotRRs)
	if len(diff) != 0 {
		t.Errorf("Got %d unexpected records in result:", len(diff))
		for _, rec := range diff {
			t.Errorf("%+v", rec)
		}
	}

	diff = difference(gotRRs, testRRs)
	if len(diff) != 0 {
		t.Errorf("Missing %d records from result:", len(diff))
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
