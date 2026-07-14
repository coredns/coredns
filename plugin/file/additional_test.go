package file

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var additionalAuth = []dns.RR{
	test.NS("example.org. 1800 IN NS ns.example.org."),
}

var additionalTestCases = []test.Case{
	{
		// Two MX records that only differ in preference share a target. Its addresses
		// belong in the additional section once.
		Qname: "mx.example.org.", Qtype: dns.TypeMX,
		Answer: []dns.RR{
			test.MX("mx.example.org. 1800 IN MX 10 host.example.org."),
			test.MX("mx.example.org. 1800 IN MX 20 host.example.org."),
		},
		Ns: additionalAuth,
		Extra: []dns.RR{
			test.A("host.example.org. 1800 IN A 192.0.2.10"),
			test.AAAA("host.example.org. 1800 IN AAAA 2001:db8::10"),
		},
	},
	{
		// SRV runs through the same additional processing.
		Qname: "srv.example.org.", Qtype: dns.TypeSRV,
		Answer: []dns.RR{
			test.SRV("srv.example.org. 1800 IN SRV 10 50 8080 host.example.org."),
			test.SRV("srv.example.org. 1800 IN SRV 20 50 8080 host.example.org."),
		},
		Ns: additionalAuth,
		Extra: []dns.RR{
			test.A("host.example.org. 1800 IN A 192.0.2.10"),
			test.AAAA("host.example.org. 1800 IN AAAA 2001:db8::10"),
		},
	},
	{
		// Distinct targets must each still be resolved.
		Qname: "two.example.org.", Qtype: dns.TypeMX,
		Answer: []dns.RR{
			test.MX("two.example.org. 1800 IN MX 10 host.example.org."),
			test.MX("two.example.org. 1800 IN MX 20 other.example.org."),
		},
		Ns: additionalAuth,
		Extra: []dns.RR{
			test.A("host.example.org. 1800 IN A 192.0.2.10"),
			test.AAAA("host.example.org. 1800 IN AAAA 2001:db8::10"),
			test.A("other.example.org. 1800 IN A 192.0.2.20"),
			test.AAAA("other.example.org. 1800 IN AAAA 2001:db8::20"),
		},
	},
}

func TestAdditionalSectionDeduplication(t *testing.T) {
	zone, err := Parse(strings.NewReader(dbAdditionalExample), testAdditionalOrigin, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{testAdditionalOrigin: zone}, Names: []string{testAdditionalOrigin}}}
	ctx := context.TODO()

	for _, tc := range additionalTestCases {
		m := tc.Msg()

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := fm.ServeDNS(ctx, rec, m)
		if err != nil {
			t.Errorf("Expected no error for %q/%d, got %v", tc.Qname, tc.Qtype, err)
			continue
		}

		resp := rec.Msg
		if err := test.SortAndCheck(resp, tc); err != nil {
			t.Errorf("Test %q/%d: %v", tc.Qname, tc.Qtype, err)
		}
	}
}

func TestAdditionalSectionDeduplicationMixedCase(t *testing.T) {
	// SRV targets are not lowercased on insert, so one target can reach additional
	// processing under two spellings. The zone's tree matches names case-insensitively,
	// so both resolve to the same addresses and those belong in the response once.
	zone, err := Parse(strings.NewReader(dbAdditionalExample), testAdditionalOrigin, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{testAdditionalOrigin: zone}, Names: []string{testAdditionalOrigin}}}
	ctx := context.TODO()

	m := new(dns.Msg)
	m.SetQuestion("mixed.example.org.", dns.TypeSRV)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := fm.ServeDNS(ctx, rec, m); err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	resp := rec.Msg
	if len(resp.Answer) != 2 {
		t.Fatalf("Expected 2 answers, got %d", len(resp.Answer))
	}
	if len(resp.Extra) != 2 {
		t.Errorf("Expected 2 additional records (one A, one AAAA), got %d: %v", len(resp.Extra), resp.Extra)
	}
}

const testAdditionalOrigin = "example.org."

const dbAdditionalExample = `
$TTL 30M
$ORIGIN example.org.
@	IN SOA	ns.example.org. admin.example.org. (
			2024010100 ; serial
			14400      ; refresh (4 hours)
			3600       ; retry (1 hour)
			604800     ; expire (1 week)
			14400      ; minimum (4 hours)
			)
	IN NS	ns.example.org.

ns		IN A	192.0.2.1

; The target shared by the records below.
host		IN A	192.0.2.10
		IN AAAA	2001:db8::10

; A second, distinct target.
other		IN A	192.0.2.20
		IN AAAA	2001:db8::20

; Two MX records differing only in preference, pointing at one target.
mx		IN MX	10 host.example.org.
		IN MX	20 host.example.org.

; Two SRV records pointing at one target.
srv		IN SRV	10 50 8080 host.example.org.
		IN SRV	20 50 8080 host.example.org.

; The same target, spelled differently. SRV targets are not normalized on insert.
mixed		IN SRV	10 50 8080 host.example.org.
		IN SRV	20 50 8080 HOST.EXAMPLE.ORG.

; Two MX records with distinct targets.
two		IN MX	10 host.example.org.
		IN MX	20 other.example.org.
`
