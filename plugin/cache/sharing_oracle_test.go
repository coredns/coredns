package cache

import (
	"fmt"
	"testing"

	"github.com/miekg/dns"
)

// The oracle is explicit DNS text, independent of sharingResponse, cache
// filtering, and any preceding response. Parsing only canonicalizes RR text.
func sharingWant(kind string, signed bool, ttl uint32) (int, []string, []string) {
	var answer, authority []string
	code := dns.RcodeSuccess
	sig := fmt.Sprintf("probe.example. %d IN RRSIG A 8 2 300 20300101000000 20200101000000 1 example. AQ==", ttl)
	soa := fmt.Sprintf("example. %d IN SOA ns.example. hostmaster.example. 1 60 60 600 300", ttl)
	switch kind {
	case "positive":
		answer = []string{fmt.Sprintf("probe.example. %d IN A 192.0.2.1", ttl)}
		if signed {
			answer = append(answer, sig)
		}
	case "cname":
		answer = []string{fmt.Sprintf("probe.example. %d IN CNAME target.example.", ttl), fmt.Sprintf("target.example. %d IN A 192.0.2.2", ttl)}
		if signed {
			answer = append(answer, sig)
		}
	case "dname":
		answer = []string{fmt.Sprintf("example. %d IN DNAME other.", ttl), fmt.Sprintf("probe.example. %d IN CNAME probe.other.", ttl), fmt.Sprintf("probe.other. %d IN A 192.0.2.3", ttl)}
		if signed {
			answer = append(answer, sig)
		}
	case "nxdomain", "nodata", "cname-nodata":
		if kind == "nxdomain" {
			code = dns.RcodeNameError
		}
		if kind == "cname-nodata" {
			answer = []string{fmt.Sprintf("probe.example. %d IN CNAME empty.example.", ttl)}
		}
		authority = []string{soa}
		if signed {
			authority = append(authority, fmt.Sprintf("probe.example. %d IN NSEC z.example. A RRSIG NSEC", ttl), sig)
		}
	case "servfail":
		code = dns.RcodeServerFailure
	case "delegation":
		authority = []string{fmt.Sprintf("probe.example. %d IN NS ns.probe.example.", ttl)}
		if signed {
			authority = append(authority, fmt.Sprintf("probe.example. %d IN DS 0 0 0", ttl))
		}
	}
	return code, answer, authority
}

func sharingAssertRecords(t *testing.T, section string, got []dns.RR, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s records=%v want=%v", section, got, want)
	}
	for n, text := range want {
		rr, err := dns.NewRR(text)
		if err != nil {
			t.Fatal(err)
		}
		if got[n].String() != rr.String() {
			t.Errorf("%s[%d]=%s want=%s", section, n, got[n], rr)
		}
	}
}

func sharingAssertResponse(t *testing.T, got *dns.Msg, kind string, signed bool, ttl uint32) {
	t.Helper()
	code, answer, authority := sharingWant(kind, signed, ttl)
	if got.Rcode != code {
		t.Errorf("rcode=%d want=%d", got.Rcode, code)
	}
	sharingAssertRecords(t, "answer", got.Answer, answer)
	sharingAssertRecords(t, "authority", got.Ns, authority)
}
