package file

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

const exampleApexOnly = `$ORIGIN example.com.
@   IN      SOA     ns1.example.com. admin.example.com.  (
                               2005011437 ; Serial
                               1200       ; Refresh
                               144        ; Retry
                               1814400    ; Expire
                               2h )       ; Minimum
@           IN  NS      ns1.example.com.
`

func TestLookupApex(t *testing.T) {
	// this tests a zone with *only* an apex. The behavior here is wrong, we should return NODATA, but we do a NXDOMAIN.
	// Adding this test to document this. Note a zone that doesn't have any data is pretty useless anyway, so rather than
	// fix this with an entirely new branch in lookup.go, just live with it.
	zone, err := Parse(strings.NewReader(exampleApexOnly), "example.com.", "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}
	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{"example.com.": zone}, Names: []string{"example.com."}}}
	ctx := context.TODO()

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := fm.ServeDNS(ctx, rec, m); err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if rec.Msg.Rcode != dns.RcodeNameError { // Should be RcodeSuccess in a perfect world.
		t.Errorf("Expected rcode %d, got %d", dns.RcodeNameError, rec.Msg.Rcode)
	}
}
