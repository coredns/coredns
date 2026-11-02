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
	// The wildcard *.went.example.org is an empty non-terminal.
	{
		Qname: "a.went.example.org.", Qtype: dns.TypeA,
		Ns: []dns.RR{
			test.SOA("example.org.	300	IN	SOA	ns.example.org. hostmaster.example.org. 2026102701 7200 3600 1209600 300"),
		},
	},
	// The CNAME target is an empty non-terminal, so the upstream answers NODATA.
	{
		Qname: "ent-alias.example.org.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.CNAME("ent-alias.example.org.	3600	IN	CNAME	went.example.org."),
		},
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

// negativeUpstream answers every lookup with an empty answer: NODATA for the names in the map, which are empty
// non-terminals in the zone, and NXDOMAIN for any other name, which does not exist in the zone.
type negativeUpstream map[string]bool

func (u negativeUpstream) Lookup(_ context.Context, _ request.Request, name string, typ uint16) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetQuestion(name, typ)
	if !u[name] {
		m.Rcode = dns.RcodeNameError
	}
	return m, nil
}

func TestLookupNegativeTTL(t *testing.T) {
	const name = "example.org."
	zone, err := Parse(strings.NewReader(dbNegativeTTL), name, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}
	zone.Upstream = negativeUpstream{"went.example.org.": true}

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

func TestNegSOA(t *testing.T) {
	soa := test.SOA("example.org.	3600	IN	SOA	ns.example.org. hostmaster.example.org. 2026102701 7200 3600 1209600 300")
	sigs := []dns.RR{
		test.RRSIG("example.org.	3600	IN	RRSIG	SOA 8 2 3600 20261127000000 20261027000000 12345 example.org. FUZSTyvZfeuuOpCm"),
		test.RRSIG("example.org.	60	IN	RRSIG	SOA 13 2 3600 20261127000000 20261027000000 54321 example.org. FUZSTyvZfeuuOpCm"),
	}
	ap := Apex{SOA: soa, SIGSOA: sigs}

	got := ap.negSOA(true)
	// The SOA and the first signature are lowered to MINIMUM; the signature with a lower TTL keeps it.
	want := []uint32{300, 300, 60}
	if len(got) != len(want) {
		t.Fatalf("Expected %d records, got %d", len(want), len(got))
	}
	for i, rr := range got {
		if rr.Header().Ttl != want[i] {
			t.Errorf("Record %d: expected TTL %d, got %d", i, want[i], rr.Header().Ttl)
		}
		if sig, ok := rr.(*dns.RRSIG); ok && sig.OrigTtl != 3600 {
			t.Errorf("Record %d: expected RRSIG original TTL to be %d, got %d", i, 3600, sig.OrigTtl)
		}
	}

	// The zone data itself is never modified.
	if soa.Hdr.Ttl != 3600 || sigs[0].Header().Ttl != 3600 || sigs[1].Header().Ttl != 60 {
		t.Errorf("Expected the apex records to keep their TTLs, got %d, %d and %d", soa.Hdr.Ttl, sigs[0].Header().Ttl, sigs[1].Header().Ttl)
	}
}

const dbNegativeTTL = `$ORIGIN example.org.
@	3600	IN	SOA	ns.example.org. hostmaster.example.org. 2026102701 7200 3600 1209600 300
	3600	IN	RRSIG	SOA 8 2 3600 20261127000000 20261027000000 12345 example.org. FUZSTyvZfeuuOpCm
	3600	IN	NS	ns.example.org.
ns	3600	IN	A	192.0.2.53
www	3600	IN	A	192.0.2.1
alias	3600	IN	CNAME	missing.example.org.
ent-alias	3600	IN	CNAME	went.example.org.
*.wild	3600	IN	TXT	"wildcard"
a.*.went	3600	IN	TXT	"below an empty non-terminal wildcard"
`
