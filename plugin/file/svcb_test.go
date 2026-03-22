package file

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var svcbAuth = []dns.RR{
	test.NS("example.com.	1800	IN	NS	ns.example.com."),
}

var svcbTestCases = []test.Case{
	{
		// Basic SVCB query with additional section glue for in-bailiwick target
		Qname: "_8443._foo.example.com.", Qtype: dns.TypeSVCB,
		Answer: []dns.RR{
			test.SVCB("_8443._foo.example.com. 1800 IN SVCB 1 svc-target.example.com. alpn=\"h2,h3\" port=\"8443\""),
		},
		Ns: svcbAuth,
		Extra: []dns.RR{
			test.A("svc-target.example.com. 1800 IN A 192.0.2.10"),
			test.AAAA("svc-target.example.com. 1800 IN AAAA 2001:db8::10"),
		},
	},
	{
		// Basic HTTPS query with additional section glue for in-bailiwick target
		Qname: "www.example.com.", Qtype: dns.TypeHTTPS,
		Answer: []dns.RR{
			test.HTTPS("www.example.com. 1800 IN HTTPS 1 svc-target.example.com. alpn=\"h2,h3\""),
		},
		Ns: svcbAuth,
		Extra: []dns.RR{
			test.A("svc-target.example.com. 1800 IN A 192.0.2.10"),
			test.AAAA("svc-target.example.com. 1800 IN AAAA 2001:db8::10"),
		},
	},
	{
		// SVCB AliasMode (Priority=0) — glue still added for in-bailiwick target
		Qname: "_alias._foo.example.com.", Qtype: dns.TypeSVCB,
		Answer: []dns.RR{
			test.SVCB("_alias._foo.example.com. 1800 IN SVCB 0 svc-target.example.com."),
		},
		Ns: svcbAuth,
		Extra: []dns.RR{
			test.A("svc-target.example.com. 1800 IN A 192.0.2.10"),
			test.AAAA("svc-target.example.com. 1800 IN AAAA 2001:db8::10"),
		},
	},
	{
		// Wildcard SVCB expansion (no additional section — wildcards don't run additionalProcessing)
		Qname: "_http._tcp.example.com.", Qtype: dns.TypeSVCB,
		Answer: []dns.RR{
			test.SVCB("_http._tcp.example.com. 1800 IN SVCB 1 svc-target.example.com. port=\"443\""),
		},
		Ns: svcbAuth,
	},
	{
		// NoData: existing name, no SVCB record
		Qname: "svc-target.example.com.", Qtype: dns.TypeSVCB,
		Ns: []dns.RR{
			test.SOA("example.com.	1800	IN	SOA	ns.example.com. admin.example.com. 2024010100 14400 3600 604800 14400"),
		},
	},
	{
		// NXDOMAIN
		Qname: "nonexistent.example.com.", Qtype: dns.TypeSVCB,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("example.com.	1800	IN	SOA	ns.example.com. admin.example.com. 2024010100 14400 3600 604800 14400"),
		},
	},
}

func TestLookupSVCB(t *testing.T) {
	zone, err := Parse(strings.NewReader(dbSVCBExample), testSVCBOrigin, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{testSVCBOrigin: zone}, Names: []string{testSVCBOrigin}}}
	ctx := context.TODO()

	for _, tc := range svcbTestCases {
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

func TestSVCBTargetNormalization(t *testing.T) {
	// Zone with mixed-case SVCB target — should be lowercased on insert
	const dbMixedCase = `
$TTL 30M
$ORIGIN example.com.
@	IN SOA ns.example.com. admin.example.com. ( 2024010100 14400 3600 604800 14400 )
	IN NS  ns.example.com.
ns	IN A   192.0.2.1
svc	IN A   192.0.2.10
_foo	IN SVCB 1 SVC.Example.COM. alpn=h2
`
	zone, err := Parse(strings.NewReader(dbMixedCase), testSVCBOrigin, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{testSVCBOrigin: zone}, Names: []string{testSVCBOrigin}}}
	ctx := context.TODO()

	m := new(dns.Msg)
	m.SetQuestion("_foo.example.com.", dns.TypeSVCB)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	_, err = fm.ServeDNS(ctx, rec, m)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	resp := rec.Msg
	if len(resp.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(resp.Answer))
	}

	svcb, ok := resp.Answer[0].(*dns.SVCB)
	if !ok {
		t.Fatalf("Expected SVCB record, got %T", resp.Answer[0])
	}
	if svcb.Target != "svc.example.com." {
		t.Errorf("Expected lowercased target %q, got %q", "svc.example.com.", svcb.Target)
	}

	// Verify additional section contains glue (target was normalized so lookup works)
	if len(resp.Extra) != 1 {
		t.Fatalf("Expected 1 extra record (A glue), got %d", len(resp.Extra))
	}
}

func TestSVCBPrivateKeys(t *testing.T) {
	// Test DNS-AID private-use SvcParamKeys (65400-65408) round-trip
	zone, err := Parse(strings.NewReader(dbSVCBExample), testSVCBOrigin, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{testSVCBOrigin: zone}, Names: []string{testSVCBOrigin}}}
	ctx := context.TODO()

	m := new(dns.Msg)
	m.SetQuestion("_dnsaid.example.com.", dns.TypeHTTPS)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	_, err = fm.ServeDNS(ctx, rec, m)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	resp := rec.Msg
	if len(resp.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(resp.Answer))
	}

	https, ok := resp.Answer[0].(*dns.HTTPS)
	if !ok {
		t.Fatalf("Expected HTTPS record, got %T", resp.Answer[0])
	}

	// Verify private-use keys survived the round-trip
	rr := https.String()
	for _, key := range []string{"key65400=", "key65401=", "key65406="} {
		if !strings.Contains(rr, key) {
			t.Errorf("Expected response to contain %s, got: %s", key, rr)
		}
	}
}

const testSVCBOrigin = "example.com."

const dbSVCBExample = `
$TTL 30M
$ORIGIN example.com.
@	IN SOA	ns.example.com. admin.example.com. (
			2024010100 ; serial
			14400      ; refresh (4 hours)
			3600       ; retry (1 hour)
			604800     ; expire (1 week)
			14400      ; minimum (4 hours)
			)
	IN NS	ns.example.com.

; A/AAAA records for glue
ns		IN A	192.0.2.1
svc-target	IN A	192.0.2.10
		IN AAAA	2001:db8::10

; SVCB ServiceMode with in-bailiwick target (tests additional section processing)
_8443._foo	IN SVCB	1 svc-target.example.com. alpn=h2,h3 port=8443

; HTTPS ServiceMode with in-bailiwick target
www		IN HTTPS 1 svc-target.example.com. alpn=h2,h3

; SVCB AliasMode (Priority=0)
_alias._foo	IN SVCB	0 svc-target.example.com.

; Wildcard SVCB
*._tcp		IN SVCB	1 svc-target.example.com. port=443

; DNS-AID private-use keys (65400-65408)
_dnsaid		IN HTTPS 1 svc-target.example.com. alpn=h2 key65400="https://aid.example.com/cap" key65401="sha256:e3b0c44298fc" key65406="apphub-psc"
`
