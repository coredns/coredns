package file

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// The SOA TTL (3600) is higher than its MINIMUM field (300), so negative answers must carry the SOA with a
// TTL of 300, see RFC 2308, Section 3.
var negativeTTLTestCases = []test.Case{
	{
		Qname: "missing.example.org.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("example.org.	300	IN	SOA	ns.example.org. hostmaster.example.org. 2026102701 7200 3600 1209600 300"),
		},
	},
	{
		Qname: "www.example.org.", Qtype: dns.TypeMX,
		Ns: []dns.RR{
			test.SOA("example.org.	300	IN	SOA	ns.example.org. hostmaster.example.org. 2026102701 7200 3600 1209600 300"),
		},
	},
	{
		Qname: "a.wild.example.org.", Qtype: dns.TypeMX,
		Ns: []dns.RR{
			test.SOA("example.org.	300	IN	SOA	ns.example.org. hostmaster.example.org. 2026102701 7200 3600 1209600 300"),
		},
	},
	{
		Qname: "alias.example.org.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Answer: []dns.RR{
			test.CNAME("alias.example.org.	3600	IN	CNAME	missing.example.org."),
		},
		Ns: []dns.RR{
			test.SOA("example.org.	300	IN	SOA	ns.example.org. hostmaster.example.org. 2026102701 7200 3600 1209600 300"),
		},
	},
	{
		Qname: "missing.example.org.", Qtype: dns.TypeA, Do: true,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.RRSIG("example.org.	300	IN	RRSIG	SOA 8 2 3600 20261127000000 20261027000000 12345 example.org. FUZSTyvZfeuuOpCm"),
			test.SOA("example.org.	300	IN	SOA	ns.example.org. hostmaster.example.org. 2026102701 7200 3600 1209600 300"),
		},
	},
	// Positive answers keep the SOA's own TTL, also after the negative answers above.
	{
		Qname: "example.org.", Qtype: dns.TypeSOA,
		Answer: []dns.RR{
			test.SOA("example.org.	3600	IN	SOA	ns.example.org. hostmaster.example.org. 2026102701 7200 3600 1209600 300"),
		},
		Ns: []dns.RR{
			test.NS("example.org.	3600	IN	NS	ns.example.org."),
		},
	},
	{
		Qname: "example.org.", Qtype: dns.TypeSOA, Do: true,
		Answer: []dns.RR{
			test.RRSIG("example.org.	3600	IN	RRSIG	SOA 8 2 3600 20261127000000 20261027000000 12345 example.org. FUZSTyvZfeuuOpCm"),
			test.SOA("example.org.	3600	IN	SOA	ns.example.org. hostmaster.example.org. 2026102701 7200 3600 1209600 300"),
		},
		Ns: []dns.RR{
			test.NS("example.org.	3600	IN	NS	ns.example.org."),
		},
	},
}

// nxdomainUpstream answers every lookup with NXDOMAIN, which is what the server returns for a CNAME target that
// does not exist in the zone.
type nxdomainUpstream struct{}

func (nxdomainUpstream) Lookup(_ context.Context, _ request.Request, name string, typ uint16) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetQuestion(name, typ)
	m.Rcode = dns.RcodeNameError
	return m, nil
}

func TestLookupNegativeTTL(t *testing.T) {
	const name = "example.org."
	zone, err := Parse(strings.NewReader(dbNegativeTTL), name, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}
	zone.Upstream = nxdomainUpstream{}

	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{name: zone}, Names: []string{name}}}
	ctx := context.TODO()

	for _, tc := range negativeTTLTestCases {
		m := tc.Msg()

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		if _, err := fm.ServeDNS(ctx, rec, m); err != nil {
			t.Errorf("Expected no error for %q/%d, got %v", tc.Qname, tc.Qtype, err)
			continue
		}

		if err := test.SortAndCheck(rec.Msg, tc); err != nil {
			t.Errorf("Test %q/%d: %v", tc.Qname, tc.Qtype, err)
		}

		// Lowering the TTL of the RRSIG must leave the original TTL it covers untouched.
		for _, rr := range append(rec.Msg.Answer, rec.Msg.Ns...) {
			if sig, ok := rr.(*dns.RRSIG); ok && sig.OrigTtl != 3600 {
				t.Errorf("Test %q/%d: expected RRSIG original TTL to be %d, got %d", tc.Qname, tc.Qtype, 3600, sig.OrigTtl)
			}
		}
	}
}

const dbNegativeTTL = `$ORIGIN example.org.
@	3600	IN	SOA	ns.example.org. hostmaster.example.org. 2026102701 7200 3600 1209600 300
	3600	IN	RRSIG	SOA 8 2 3600 20261127000000 20261027000000 12345 example.org. FUZSTyvZfeuuOpCm
	3600	IN	NS	ns.example.org.
ns	3600	IN	A	192.0.2.53
www	3600	IN	A	192.0.2.1
alias	3600	IN	CNAME	missing.example.org.
*.wild	3600	IN	TXT	"wildcard"
`
