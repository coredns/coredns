package kubernetes

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestKubernetesXFR(t *testing.T) {
	k := New([]string{"example.com."})
	k.APIConn = &APIConnServeTest{}
	k.TransferTo = []string{"10.240.0.1:53"}
	k.Namespaces = map[string]bool{"testns": true}

	ctx := context.TODO()
	w := dnstest.NewMultiRecorder(&test.ResponseWriter{})
	dnsmsg := &dns.Msg{}
	dnsmsg.SetAxfr(k.Zones[0])

	_, err := k.ServeDNS(ctx, w, dnsmsg)
	if err != nil {
		t.Error(err)
	}

	if len(w.Msgs) == 0 {
		t.Logf("%+v\n", w)
		t.Fatal("Did not get back a zone response")
	}

	if len(w.Msgs[0].Answer) == 0 {
		t.Logf("%+v\n", w)
		t.Fatal("Did not get back an answer")
	}

	// Ensure xfr starts with SOA
	if w.Msgs[0].Answer[0].Header().Rrtype != dns.TypeSOA {
		t.Error("Invalid XFR, does not start with SOA record")
	}

	// Ensure xfr starts with SOA
	// Last message is empty, so we need to go back one further
	if w.Msgs[len(w.Msgs)-2].Answer[len(w.Msgs[len(w.Msgs)-2].Answer)-1].Header().Rrtype != dns.TypeSOA {
		t.Error("Invalid XFR, does not end with SOA record")
	}

	testRRs := []dns.RR{}
	for _, tc := range dnsTestCases {
		if tc.Rcode != dns.RcodeSuccess {
			continue
		}

		for _, ans := range tc.Answer {
			// Exclude wildcard searches
			if strings.Contains(ans.Header().Name, "*") {
				continue
			}

			// Exclude TXT records
			if ans.Header().Rrtype == dns.TypeTXT {
				continue
			}
			testRRs = append(testRRs, ans)
		}
	}

	gotRRs := []dns.RR{}
	for _, resp := range w.Msgs {
		for _, ans := range resp.Answer {
			// Skip SOA records since these
			// test cases do not exist
			if ans.Header().Rrtype == dns.TypeSOA {
				continue
			}

			gotRRs = append(gotRRs, ans)
		}

	}

	diff := difference(testRRs, gotRRs)
	if len(diff) != 0 {
		t.Errorf("Got back %d records that do not exist in test cases, should be 0:", len(diff))
		for _, rec := range diff {
			t.Errorf("%+v", rec)
		}
	}

	diff = difference(gotRRs, testRRs)
	if len(diff) != 0 {
		t.Errorf("Found %d records we're missing, should be 0:", len(diff))
		for _, rec := range diff {
			t.Errorf("%+v", rec)
		}
	}
}

func TestKubernetesXFRNotAllowed(t *testing.T) {
	k := New([]string{"example.com."})
	k.APIConn = &APIConnServeTest{}
	k.TransferTo = []string{"1.2.3.4:53"}
	k.Namespaces = map[string]bool{"testns": true}

	ctx := context.TODO()
	w := dnstest.NewMultiRecorder(&test.ResponseWriter{})
	dnsmsg := &dns.Msg{}
	dnsmsg.SetAxfr(k.Zones[0])

	_, err := k.ServeDNS(ctx, w, dnsmsg)
	if err != nil {
		t.Error(err)
	}

	if len(w.Msgs) == 0 {
		t.Logf("%+v\n", w)
		t.Fatal("Did not get back a zone response")
	}

	if len(w.Msgs[0].Answer) != 0 {
		t.Logf("%+v\n", w)
		t.Fatal("Got an answer, should not have")
	}
}

// difference shows what we're missing when comparing two RR slices
func difference(testRRs []dns.RR, gotRRs []dns.RR) []dns.RR {
	expectedRRs := map[string]bool{}
	for _, rr := range testRRs {
		expectedRRs[rr.String()] = true
	}

	foundRRs := []dns.RR{}
	for _, rr := range gotRRs {
		if _, ok := expectedRRs[rr.String()]; !ok {
			foundRRs = append(foundRRs, rr)
		}
	}
	return foundRRs
}