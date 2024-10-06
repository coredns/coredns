package test

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/rlc"
	"github.com/coredns/coredns/plugin/test"
	"github.com/golang/groupcache"
	"github.com/miekg/dns"
	"google.golang.org/protobuf/proto"
)

const hrefNET = `; example.org test file
$TTL 3600
@            IN  SOA    sns.dns.icann.org. noc.dns.icann.org. 2024010941 7200 3600 1209600 3600
@            IN  NS     b.iana-servers.net.
@            IN  NS     a.iana-servers.net.
@            IN  A      127.0.0.1
@            IN  A      127.0.0.2
short   1    IN  A      127.0.0.3

www			 IN  A 		127.0.0.10
www 		 IN  AAAA   2001:0db8:0000:0000:0000:0000:1428:57a0
a			 IN  A 		127.0.1.1
b			 IN  A 		127.0.1.2
b 			IN  AAAA    2001:0db8:0000:0000:0000:0000:1428:57ab
c			 IN  A 		127.0.1.3
d			 IN  A 		127.0.1.4
e			 IN  A 		127.0.1.5
f			 IN  A 		127.0.1.6
g			 IN  A 		127.0.1.7
h			 IN  A 		127.0.1.8
i			 IN  A 		127.0.1.9
j			 IN  A 		127.0.1.10
cname        IN  CNAME  www.example.net.
cname        IN  CNAME  www.example.org.
service      IN  SRV    8080 10 10 @
`

func TestLookupRlc(t *testing.T) {
	// Start auth. CoreDNS holding the auth zone.
	name, rm, err := test.TempFile(".", hrefNET)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()
	corefile := `example.org:0 {
		file ` + name + `
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	// Start caching forward CoreDNS that we want to test.
	corefile = `.:0 {
		forward . ` + udp + `
		rlc { 
			ttl 3600
			capacity 100
		}
	}`

	i, udp, _, err = CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	cacheTestCases := []struct {
		name               string
		query              string // the test message going "in" to cache
		expect             []string
		shouldReverseCache bool
		exppectIpv6        bool
	}{
		{
			name:               "test reverse after forward lookup example.org.",
			expect:             []string{"127.0.0.10", "2001:0db8:0000:0000:0000:0000:1428:57a0"},
			query:              "www.example.org.",
			shouldReverseCache: true,
			exppectIpv6:        true,
		},
		{
			name:               "test reverse after forward lookup example.org.",
			expect:             []string{"127.0.1.1"},
			query:              "a.example.org.",
			shouldReverseCache: true,
			exppectIpv6:        false,
		},
		{
			name:               "test reverse after forward lookup example.org.",
			expect:             []string{"2001:0db8:0000:0000:0000:0000:1428:57ab"},
			query:              "b.example.org.",
			shouldReverseCache: true,
			exppectIpv6:        true,
		},
	}

	for _, tc := range cacheTestCases {
		t.Run(tc.name, func(t *testing.T) {
			reversed, err := rlcGetRevers(t, udp, tc.expect)
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			if len(reversed) != 0 {
				t.Fatal("Expected no reverse lookup results")
			}

			m := new(dns.Msg)
			m.SetQuestion(tc.query, dns.TypeA)
			_, err = dns.Exchange(m, udp)
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			if tc.exppectIpv6 {
				m.SetQuestion(tc.query, dns.TypeAAAA)
				_, err = dns.Exchange(m, udp)
				if err != nil {
					t.Fatalf("Expected to receive reply, but didn't: %s", err)
				}
			}

			reversed, err = rlcGetRevers(t, udp, tc.expect)
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			if len(reversed) == 0 {
				t.Fatal("Expected reverse lookup results")
			}
			for _, expected := range tc.expect {
				if reversed[expected] != tc.query {
					t.Fatal("Expected reverse lookup did not contain expected result")
				}

			}
		})
	}
}

func TestCapacityRlc(t *testing.T) {
	// Start auth. CoreDNS holding the auth zone.
	name, rm, err := test.TempFile(".", hrefNET)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	corefile := `example.org:0 {
		file ` + name + `
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	// Start caching forward CoreDNS that we want to test.
	corefile = `.:0 {
		forward . ` + udp + `
		rlc { 
			ttl 3600
			capacity 2
		}
	}`

	i, udp, _, err = CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	for i := 0; i < 10; i++ {
		query := fmt.Sprintf("%c.example.org.", 'a'+i)
		expected := fmt.Sprintf("127.0.1.%d", 1+i)
		t.Run("cache "+query, func(t *testing.T) {
			reversed, err := rlcGetRevers(t, udp, []string{expected})
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			if len(reversed) != 0 {
				t.Fatal("Expected no reverse lookup results")
			}

			m := new(dns.Msg)
			m.SetQuestion(query, dns.TypeA)
			_, err = dns.Exchange(m, udp)
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			reversed, err = rlcGetRevers(t, udp, []string{expected})
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			if len(reversed) == 0 {
				t.Fatal("Expected reverse lookup results")
			}
			if reversed[expected] != query {
				t.Fatal("Expected reverse lookup to match")
			}
		})
	}

	expectedValid := 2
	for i := 9; i > -1; i-- {
		query := fmt.Sprintf("%c.example.org.", 'a'+i)
		expected := fmt.Sprintf("127.0.1.%d", 1+i)
		t.Run("check cache status for "+query, func(t *testing.T) {
			reversed, err := rlcGetRevers(t, udp, []string{expected})
			if expectedValid > 0 {
				if err != nil {
					t.Fatalf("Expected to receive reply, but didn't: %s", err)
				}
				if len(reversed) == 0 {
					t.Fatal("Expected reverse lookup results")
				}
				if reversed[expected] != query {
					t.Fatal("Expected reverse lookup to match")
				}
				expectedValid = expectedValid - 1
			} else {
				if err != nil {
					t.Fatalf("Expected to receive reply, but didn't: %s", err)
				}
				if len(reversed) != 0 {
					t.Fatal("Expected no reverse lookup results")
				}
			}
		})
	}
}

func TestTTLRlc(t *testing.T) {
	// Start auth. CoreDNS holding the auth zone.
	name, rm, err := test.TempFile(".", hrefNET)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	corefile := `example.org:0 {
		file ` + name + `
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	// Start caching forward CoreDNS that we want to test.
	corefile = `.:0 {
		forward . ` + udp + `
		rlc { 
			ttl 5
			capacity 100
		}
	}`

	i, udp, _, err = CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	for i := 0; i < 10; i++ {
		query := fmt.Sprintf("%c.example.org.", 'a'+i)
		expected := fmt.Sprintf("127.0.1.%d", 1+i)
		t.Run("cache "+query, func(t *testing.T) {
			reversed, err := rlcGetRevers(t, udp, []string{expected})
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			if len(reversed) != 0 {
				t.Fatal("Expected no reverse lookup results")
			}

			m := new(dns.Msg)
			m.SetQuestion(query, dns.TypeA)
			_, err = dns.Exchange(m, udp)
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			reversed, err = rlcGetRevers(t, udp, []string{expected})
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			if len(reversed) == 0 {
				t.Fatal("Expected reverse lookup results")
			}
			if reversed[expected] != query {
				t.Fatal("Expected reverse lookup to match")
			}
		})
	}

	time.Sleep(1 * time.Second)
	// entries should still be there
	for i := 9; i > -1; i-- {
		query := fmt.Sprintf("%c.example.org.", 'a'+i)
		expected := fmt.Sprintf("127.0.1.%d", 1+i)
		t.Run("check cache status for "+query, func(t *testing.T) {
			reversed, err := rlcGetRevers(t, udp, []string{expected})
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			if len(reversed) == 0 {
				t.Fatal("Expected reverse lookup results")
			}
			if reversed[expected] != query {
				t.Fatal("Expected reverse lookup to match")
			}
		})
	}

	time.Sleep(6 * time.Second)
	// entries should be gone by now
	for i := 9; i > -1; i-- {
		query := fmt.Sprintf("%c.example.org.", 'a'+i)
		expected := fmt.Sprintf("127.0.1.%d", 1+i)
		t.Run("check cache status for "+query, func(t *testing.T) {
			reversed, err := rlcGetRevers(t, udp, []string{expected})
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			if len(reversed) != 0 {
				t.Fatal("Expected no reverse lookup results")
			}
		})
	}
}

func TestGroupcacheRlc(t *testing.T) {
	// Start auth. CoreDNS holding the auth zone.
	name, rm, err := test.TempFile(".", hrefNET)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()
	// since the cache group is global, make sure names are unique within
	corefile := `example.net:0 {
		file ` + name + `
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal("faild to get port for cache", err)
	}
	parts := strings.Split(l.Addr().String(), ":")
	peerCachePort := parts[len(parts)-1]
	l.Close()

	// Start caching forward CoreDNS that we want to test.
	corefile = `.:0 {
		forward . ` + udp + `
		rlc { 
			ttl 3600
			capacity 100
			groupcache ` + peerCachePort + `
		}
	}`

	i, udp, _, err = CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	for i := 0; i < 5; i++ {
		query := fmt.Sprintf("%c.example.net.", 'a'+i)
		expected := fmt.Sprintf("127.0.1.%d", 1+i)
		t.Run("cache "+query, func(t *testing.T) {
			reversed, err := rlcGetRevers(t, udp, []string{expected})
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			if len(reversed) != 0 {
				t.Fatal("Expected no reverse lookup results")
			}

			m := new(dns.Msg)
			m.SetQuestion(query, dns.TypeA)
			_, err = dns.Exchange(m, udp)
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			reversed, err = rlcGetRevers(t, udp, []string{expected})
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			if len(reversed) == 0 {
				t.Fatal("Expected reverse lookup results")
			}
			if reversed[expected] != query {
				t.Fatal("Expected reverse lookup to match")
			}
		})
	}
	group := groupcache.GetGroup("rlc")
	ctx := context.Background()
	validExpected := 5
	// entries should still be there
	for i := 0; i < 10; i++ {
		expected := fmt.Sprintf("%c.example.net.", 'a'+i)
		query := fmt.Sprintf("127.0.1.%d", 1+i)
		t.Run("check cache status for "+query, func(t *testing.T) {
			var result string
			sink := groupcache.StringSink(&result)
			err := group.Get(ctx, query, sink)
			if validExpected > 0 {
				if err != nil {
					t.Fatalf("Expected to receive reply, but didn't: %s", err)
				}
				if result == "" {
					t.Fatal("Expected cache results")
				}

				b := []byte(result)
				var ptr rlc.PTREntry
				err = proto.Unmarshal(b, &ptr)
				if err != nil {
					t.Fatalf("unmarshall failed, but didn't: %s", err)
				}
				if len(ptr.Names) != 1 {
					t.Fatal("Expected cache lookup have one result")
				}
				if ptr.Names[expected] == nil {
					t.Fatal("Expected cache lookup result to be valid")
				}
				if ptr.Names[expected].Name == query {
					t.Fatal("Expected cache lookup result to match expected value")
				}
				validExpected = validExpected - 1
			} else {
				if err != nil {
					t.Fatalf("Expected to receive reply, but didn't: %s", err)
				}
				if result != "" {
					t.Fatal("Expected no cache results")
				}

			}
		})
	}
}

// coredns dnstap is send before answers are added to the message and thus are (currently) no valid test case
func _TestRemoteRlc(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal("faild to get port for cache", err)
	}
	parts := strings.Split(l.Addr().String(), ":")
	remotePort := parts[len(parts)-1]
	l.Close()

	// Start auth. CoreDNS holding the auth zone.
	name, rm, err := test.TempFile(".", hrefNET)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	corefile := `example.org:0 {
		file ` + name + `
		dnstap_asnwer tcp://127.0.0.1:` + remotePort + ` full 
	}`

	i, upstream, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	// Start caching forward CoreDNS that we want to test.
	corefile = `.:0 {
		rlc { 
			ttl 3600
			capacity 100
			remote ` + remotePort + `	
		}
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	for i := 0; i < 10; i++ {
		query := fmt.Sprintf("%c.example.org.", 'a'+i)
		expected := fmt.Sprintf("127.0.1.%d", 1+i)
		t.Run("cache "+query, func(t *testing.T) {
			// make sure, the ptrs do not resolve locall

			reversed, err := rlcGetRevers(t, udp, []string{expected})
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			if len(reversed) != 0 {
				t.Fatal("Expected no reverse lookup results")
			}

			// query on server with dnstap sending to test instance
			m := new(dns.Msg)
			m.SetQuestion(query, dns.TypeA)
			res, err := dns.Exchange(m, upstream)
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}

			if len(res.Answer) != 1 {
				t.Fatal("Expected one answer from remote")
			}
			if res.Answer[0].(*dns.A).A.String() != expected {
				t.Fatal("Received answer was not expected")
			}
		})
	}

	// wait some time for dnstap to dequeue all data
	time.Sleep(5 * time.Second)
	for i := 0; i < 10; i++ {
		query := fmt.Sprintf("%c.example.org.", 'a'+i)
		expected := fmt.Sprintf("127.0.1.%d", 1+i)
		t.Run("retreive "+query, func(t *testing.T) {
			// check if its now locally available
			reversed, err := rlcGetRevers(t, udp, []string{expected})
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %s", err)
			}
			if len(reversed) == 0 {
				t.Fatal("Expected reverse lookup results")
			}
			if reversed[expected] != query {
				t.Fatal("Expected reverse lookup to match")
			}
		})
	}
}

func rlcGetRevers(t *testing.T, srv string, addrs []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, addr := range addrs {
		var query string
		parts := strings.Split(addr, ".")
		if len(parts) < 4 {
			parts = strings.Split(addr, ":")
		}
		slices.Reverse(parts)
		query = strings.Join(parts, ".")
		if len(parts) == 4 {
			query = query + ".in-addr.arpa."
		} else {
			query = query + ".ip6.arpa."
		}
		m := new(dns.Msg)
		m.SetQuestion(query, dns.TypePTR)

		resp, err := dns.Exchange(m, srv)
		if err != nil {
			return nil, err
		}
		if len(resp.Answer) > 0 {
			result[addr] = resp.Answer[0].(*dns.PTR).Ptr
		}
	}
	return result, nil
}