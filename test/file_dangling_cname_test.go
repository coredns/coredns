package test

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// Zone with a CNAME whose target does not exist in the zone.
const exampleOrgDanglingCNAME = `; example.org test file
$TTL 3600
@       IN  SOA   sns.dns.icann.org. noc.dns.icann.org. 2015082541 7200 3600 1209600 3600
@       IN  NS    b.iana-servers.net.
name    IN  CNAME name2.example.org.
`

// A negative (NXDOMAIN) answer produced by following a CNAME to a
// non-existent in-zone name must carry the zone SOA in the authority
// section, per RFC 2308, just like querying that name directly.
// See https://github.com/coredns/coredns/issues/6385.
func TestZoneDanglingCNAMENegativeAuthority(t *testing.T) {
	t.Parallel()

	zoneFile, rm, err := test.TempFile(".", exampleOrgDanglingCNAME)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	corefile := `example.org:0 {
		file ` + zoneFile + `
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("name.example.org.", dns.TypeA)
	resp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("Expected to receive reply, but didn't: %s", err)
	}

	if resp.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN, got %s", dns.RcodeToString[resp.Rcode])
	}
	if len(resp.Ns) != 1 {
		t.Fatalf("Expected 1 record in authority section, got %d: %v", len(resp.Ns), resp.Ns)
	}
	if _, ok := resp.Ns[0].(*dns.SOA); !ok {
		t.Errorf("Expected SOA in authority section, got %T: %v", resp.Ns[0], resp.Ns[0])
	}
}
