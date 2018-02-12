package dnsserver

import (
	"net"
	"testing"
)

func TestNormalizeZone(t *testing.T) {
	for i, test := range []struct {
		input     string
		expected  string
		shouldErr bool
	}{
		{".", "dns://.:53", false},
		{".:54", "dns://.:54", false},
		{"..", "://:", true},
		{"..", "://:", true},
		{".:", "://:", true},
	} {
		addr, err := normalizeZone(test.input)
		if addr == nil {
			addr = &ZoneAddr{}
		}
		actual := addr.String()
		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error, but there wasn't any", i)
		}
		if !test.shouldErr && err != nil {
			t.Errorf("Test %d: Expected no error, but there was one: %v", i, err)
		}
		if actual != test.expected {
			t.Errorf("Test %d: Expected %s but got %s", i, test.expected, actual)
		}
	}
}

func TestNormalizeZoneReverse(t *testing.T) {
	for i, test := range []struct {
		input     string
		expected  string
		shouldErr bool
	}{
		{"2003::1/64", "dns://0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.2.ip6.arpa.:53", false},
		{"2003::1/64.", "dns://2003::1/64.:53", false}, // OK, with closing dot the parse will fail.
		{"2003::1/64:53", "dns://0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.2.ip6.arpa.:53", false},
		{"2003::1/64.:53", "dns://2003::1/64.:53", false},

		{"10.0.0.0/24", "dns://0.0.10.in-addr.arpa.:53", false},
		{"10.0.0.0/24.", "dns://10.0.0.0/24.:53", false},
		{"10.0.0.0/24:53", "dns://0.0.10.in-addr.arpa.:53", false},
		{"10.0.0.0/24.:53", "dns://10.0.0.0/24.:53", false},

		// non %8==0 netmasks
		{"2003::53/67", "dns://0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.2.ip6.arpa.:53", false},
		{"10.0.0.0/25.", "dns://10.0.0.0/25.:53", false}, // has dot
		{"10.0.0.0/25", "dns://0.0.10.in-addr.arpa.:53", false},
		{"fd00:77:30::0/110", "dns://0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.7.7.0.0.0.0.d.f.ip6.arpa.:53", false},
	} {
		addr, err := normalizeZone(test.input)
		if addr == nil {
			addr = &ZoneAddr{}
		}
		actual := addr.String()
		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error, but there wasn't any", i)
		}
		if !test.shouldErr && err != nil {
			t.Errorf("Test %d: Expected no error, but there was one: %v", i, err)
		}
		if actual != test.expected {
			t.Errorf("Test %d: Expected %s but got %s", i, test.expected, actual)
		}
	}
}

type checkCall struct {
	zone       string
	same       bool
	overlap    bool
	overlapKey string
}

type checkTest struct {
	sequence []checkCall
}

func TestOverlapAddressChecker(t *testing.T) {
	for i, test := range []checkTest{
		{sequence: []checkCall{
			{"dns://.:53", false, false, ""},
			{"dns://.:53", true, false, ""},
		},
		},
		{sequence: []checkCall{
			{"dns://.:53", false, false, ""},
			{"dns://.:54", false, false, ""},
			{"dns://.:127.0.0.1:53", false, true, "dns://.:53"},
		},
		},
		{sequence: []checkCall{
			{"dns://.:127.0.0.1:53", false, false, ""},
			{"dns://.:54", false, false, ""},
			{"dns://.:127.0.0.1:53", true, false, ""},
		},
		},
		{sequence: []checkCall{
			{"dns://.:127.0.0.1:53", false, false, ""},
			{"dns://.:54", false, false, ""},
			{"dns://.:128.0.0.1:53", false, false, ""},
			{"dns://.:129.0.0.1:53", false, false, ""},
			{"dns://.:53", false, true, "dns://.:127.0.0.1:53"},
		},
		},
	} {

		checker := newZoneAddrOverlapValidator()
		for _, call := range test.sequence {
			za, err := parseFromKey(call.zone)
			if err != nil {
				t.Errorf("Test %d: error at normalizing zone %s, err = %v", i, call.zone, err)
			}
			same, overlap, overkey := checker.registerAndCheck(*za)
			if same != call.same {
				t.Errorf("Test %d: error, for zone %s, 'same' (%v) has not the expected value (%v)", i, call.zone, same, call.same)
			}
			if !same {
				if overlap != call.overlap {
					t.Errorf("Test %d: error, for zone %s, 'overlap' (%v) has not the expected value (%v)", i, call.zone, overlap, call.overlap)
				}
				if overlap {
					if overkey != call.overlapKey {
						t.Errorf("Test %d: error, for zone %s, 'overlap Key' (%v) has not the expected value (%v)", i, call.zone, overkey, call.overlapKey)
					}

				}
			}

		}
	}
}

func TestParseAddress(t *testing.T) {
	_, ipnet1, _ := net.ParseCIDR("2003::53/67")
	for i, test := range []ZoneAddr{
		{TransportDNS, ".", "52", "127.0.0.1", nil, map[string]string{}},
		{TransportDNS, ".", "53", "127.0.0.1", nil, map[string]string{}},
		{TransportDNS, "local.com.", "54", "127.0.0.1", nil, map[string]string{}},
		{TransportDNS, "local.com.", "54", "[::1]", nil, map[string]string{}},
		{TransportDNS, "0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.2.ip6.arpa.", "53", "[::1]", ipnet1, map[string]string{}},
	} {
		zonePrinted := test.asKey()
		addr, err := parseFromKey(zonePrinted)
		if err != nil {
			t.Errorf("Test %d: Expected no error, but there was one: %v", i, err)
		}
		secondPrint := addr.asKey()
		if zonePrinted != secondPrint {
			t.Errorf("Test %d: Second print does not match after parsing : 1 = '%v', 2 = '%v'", i, zonePrinted, secondPrint)
		}
	}
}
