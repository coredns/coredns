package loadbalance

import (
	"net"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestSortPreferred(t *testing.T) {
	records := []dns.RR{
		test.A("example.org. 300 IN A 10.9.30.1"),
		test.A("example.org. 300 IN A 10.9.20.5"),
		test.A("example.org. 300 IN A 192.168.1.2"),
		test.A("example.org. 300 IN A 10.10.0.1"),
		test.A("example.org. 300 IN A 10.9.20.3"),
		test.A("example.org. 300 IN A 172.16.0.1"),
		test.AAAA("example.org. 300 IN AAAA 2001:db8::1"),
		test.AAAA("example.org. 300 IN AAAA 2001:db8:abcd::1"),
		test.AAAA("example.org. 300 IN AAAA fd00::1"),
		test.CNAME("example.org. 300 IN CNAME alias.example.org."),
	}

	cidrs := []string{"2001:db8::/32", "10.9.20.0/24", "10.9.30.0/24"}
	subnets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, subnet, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatalf("Failed to parse CIDR: %v", err)
		}
		subnets = append(subnets, subnet)
	}

	msg := &dns.Msg{Answer: records}
	reorderPreferredSubnets(msg, subnets)
	sorted := msg.Answer

	expectedOrder := []string{
		"alias.example.org.",
		"2001:db8::1",
		"2001:db8:abcd::1",
		"10.9.20.5",
		"10.9.20.3",
		"10.9.30.1",
		"192.168.1.2",
		"10.10.0.1",
		"172.16.0.1",
		"fd00::1",
	}

	if len(sorted) != len(expectedOrder) {
		t.Fatalf("Expected %d records, got %d", len(expectedOrder), len(sorted))
	}

	for i, rr := range sorted {
		expected := expectedOrder[i]
		switch r := rr.(type) {
		case *dns.CNAME:
			if r.Target != expected {
				t.Errorf("Record %d: expected CNAME %s, got %s", i, expected, r.Target)
			}
		case *dns.A:
			if r.A.String() != expected {
				t.Errorf("Record %d: expected A IP %s, got %s", i, expected, r.A.String())
			}
		case *dns.AAAA:
			if r.AAAA.String() != expected {
				t.Errorf("Record %d: expected AAAA IP %s, got %s", i, expected, r.AAAA.String())
			}
		default:
			t.Errorf("Record %d: unexpected RR type %T", i, r)
		}
	}
}

func TestExtractIP(t *testing.T) {
	a := test.A("example.org. 300 IN A 10.0.0.1")
	ip := extractIP(a)
	if ip.String() != "10.0.0.1" {
		t.Errorf("Expected 10.0.0.1, got %s", ip.String())
	}

	aaaa := test.AAAA("example.org. 300 IN AAAA ::1")
	ip = extractIP(aaaa)
	if ip.String() != "::1" {
		t.Errorf("Expected ::1, got %s", ip.String())
	}

	cname := test.CNAME("example.org. 300 IN CNAME other.org.")
	ip = extractIP(cname)
	if ip != nil {
		t.Errorf("Expected nil for CNAME, got %v", ip)
	}
}
