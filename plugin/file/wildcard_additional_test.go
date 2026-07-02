package file

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// Wildcard-synthesized answers must run additional-section processing for
// MX/SRV/SVCB/HTTPS targets, just like non-wildcard answers do.
// See https://github.com/coredns/coredns/issues/6629.
const dbWildcardAdditional = `
$TTL 30M
$ORIGIN example.org.
@	IN SOA	ns.example.org. admin.example.org. 2024010100 14400 3600 604800 14400
	IN NS	ns.example.org.
ns	IN A	192.0.2.1
mail	IN A	192.0.2.10
	IN AAAA	2001:db8::10
*.wild	IN MX	10 mail.example.org.
`

const testWildcardAdditionalOrigin = "example.org."

var wildcardAdditionalTestCases = []test.Case{
	{
		// Wildcard-synthesized MX answer includes glue (A/AAAA) for the
		// in-bailiwick target in the additional section.
		Qname: "foo.wild.example.org.", Qtype: dns.TypeMX,
		Answer: []dns.RR{
			test.MX("foo.wild.example.org. 1800 IN MX 10 mail.example.org."),
		},
		Ns: []dns.RR{
			test.NS("example.org. 1800 IN NS ns.example.org."),
		},
		Extra: []dns.RR{
			test.A("mail.example.org. 1800 IN A 192.0.2.10"),
			test.AAAA("mail.example.org. 1800 IN AAAA 2001:db8::10"),
		},
	},
}

func TestLookupWildcardAdditional(t *testing.T) {
	zone, err := Parse(strings.NewReader(dbWildcardAdditional), testWildcardAdditionalOrigin, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{testWildcardAdditionalOrigin: zone}, Names: []string{testWildcardAdditionalOrigin}}}
	ctx := context.TODO()

	for _, tc := range wildcardAdditionalTestCases {
		m := tc.Msg()

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		if _, err := fm.ServeDNS(ctx, rec, m); err != nil {
			t.Errorf("Expected no error for %q/%d, got %v", tc.Qname, tc.Qtype, err)
			continue
		}

		if err := test.SortAndCheck(rec.Msg, tc); err != nil {
			t.Errorf("Test %q/%d: %v", tc.Qname, tc.Qtype, err)
		}
	}
}
