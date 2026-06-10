package file

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

const dbEscapeOwner = `campus.edu.              500 IN SOA ns1.outside.edu. root.campus.edu. 8 6048 4000 2419200 6048
campus.edu.              500 IN NS  ns1.outside.edu.
has\046dot.campus.edu.   500 IN A   192.0.2.2
`

// TestLookupOwnerNameWithDecimalEscape covers RFC 1035 §5.1 \DDD escape
// notation in owner names. The miekg/dns parser preserves whichever text
// form the zone file used (\046 vs \.), but incoming queries arrive as
// the canonical wire-unpacked form (\.). Without normalization the two
// strings don't compare equal and the record is silently unreachable.
func TestLookupOwnerNameWithDecimalEscape(t *testing.T) {
	const origin = "campus.edu."
	zone, err := Parse(strings.NewReader(dbEscapeOwner), origin, "stdin", 0)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{origin: zone}, Names: []string{origin}}}
	ctx := context.TODO()

	// The wire form of the owner name is h-a-s-.-d-o-t (one label,
	// seven bytes), which UnpackDomainName turns into "has\.dot".
	tc := test.Case{
		Qname: `has\.dot.campus.edu.`, Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A(`has\.dot.campus.edu.	500	IN	A	192.0.2.2`),
		},
		Ns: []dns.RR{
			test.NS(`campus.edu.	500	IN	NS	ns1.outside.edu.`),
		},
	}

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := fm.ServeDNS(ctx, rec, tc.Msg()); err != nil {
		t.Fatalf("ServeDNS: %v", err)
	}
	if err := test.SortAndCheck(rec.Msg, tc); err != nil {
		t.Error(err)
	}
}

func TestCanonicalEscape(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`has\046dot.campus.edu.`, `has\.dot.campus.edu.`},
		{`has\.dot.campus.edu.`, `has\.dot.campus.edu.`},
		{`plain.campus.edu.`, `plain.campus.edu.`},
		{`a\009b.campus.edu.`, `a\009b.campus.edu.`}, // tab stays \DDD (unprintable)
	}
	for _, tc := range cases {
		if got := canonicalEscape(tc.in); got != tc.want {
			t.Errorf("canonicalEscape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
