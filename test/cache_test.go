package test

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestLookupCache(t *testing.T) {
	// Start auth. CoreDNS holding the auth zone.
	name, rm, err := test.TempFile(".", exampleOrg)
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
	corefile = `example.org:0 {
		forward . ` + udp + `
		cache 10
	}`

	i, udp, _, err = CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	t.Run("Long TTL", func(t *testing.T) {
		testCase(t, "example.org.", udp, 2, 10)
	})

	t.Run("Short TTL", func(t *testing.T) {
		testCase(t, "short.example.org.", udp, 1, 5)
	})

	t.Run("DNSSEC OPT", func(t *testing.T) {
		testCaseDNSSEC(t, "example.org.", udp, 4096)
	})

	t.Run("DNSSEC OPT", func(t *testing.T) {
		testCaseDNSSEC(t, "example.org.", udp, 0)
	})
}

func testCase(t *testing.T, name, addr string, expectAnsLen int, expectTTL uint32) {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeA)
	resp, err := dns.Exchange(m, addr)
	if err != nil {
		t.Fatalf("Expected to receive reply, but didn't: %s", err)
	}

	if len(resp.Answer) != expectAnsLen {
		t.Fatalf("Expected %v RR in the answer section, got %v.", expectAnsLen, len(resp.Answer))
	}

	ttl := resp.Answer[0].Header().Ttl
	if ttl != expectTTL {
		t.Errorf("Expected TTL to be %d, got %d", expectTTL, ttl)
	}
}

func testCaseDNSSEC(t *testing.T, name, addr string, bufsize int) {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeA)

	if bufsize > 0 {
		o := &dns.OPT{Hdr: dns.RR_Header{Name: ".", Rrtype: dns.TypeOPT}}
		o.SetDo()
		o.SetUDPSize(uint16(bufsize))
		m.Extra = append(m.Extra, o)
	}
	resp, err := dns.Exchange(m, addr)
	if err != nil {
		t.Fatalf("Expected to receive reply, but didn't: %s", err)
	}

	if len(resp.Extra) == 0 && bufsize == 0 {
		// no OPT, this is OK
		return
	}

	opt := resp.Extra[len(resp.Extra)-1]
	if x, ok := opt.(*dns.OPT); !ok && bufsize > 0 {
		t.Fatalf("Expected OPT RR, got %T", x)
	}
	if bufsize > 0 {
		if !opt.(*dns.OPT).Do() {
			t.Errorf("Expected DO bit to be set, got false")
		}
		if x := opt.(*dns.OPT).UDPSize(); int(x) != bufsize {
			t.Errorf("Expected %d bufsize, got %d", bufsize, x)
		}
	} else {
		if opt.Header().Rrtype == dns.TypeOPT {
			t.Errorf("Expected no OPT RR, but got one: %s", opt)
		}
	}
}

func TestLookupCacheWithoutEdns(t *testing.T) {
	name, rm, err := test.TempFile(".", exampleOrg)
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
	corefile = `example.org:0 {
		forward . ` + udp + `
		cache 10
	}`

	i, udp, _, err = CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	resp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("Expected to receive reply, but didn't: %s", err)
	}
	if len(resp.Extra) == 0 {
		return
	}

	if resp.Extra[0].Header().Rrtype == dns.TypeOPT {
		t.Fatalf("Expected no OPT RR, but got: %s", resp.Extra[0])
	}
	t.Fatalf("Expected empty additional section, got %v", resp.Extra)
}

// TestIssue7630 exercises the metadata map race reported in GitHub issue #7630.
// It configures a server chain: metadata -> log -> forward -> cache with serve_stale
// and very short TTLs to force prefetch. The test primes the cache, lets entries expire,
// then sends concurrent queries mixing positive and NXDOMAIN lookups while the log plugin
// formats lines that repeatedly read the {/forward/upstream} metadata key.
//
// Without the fix in cache.doPrefetch that creates a fresh metadata context for the
// background prefetch goroutine, this scenario can crash with:
//
//	"fatal error: concurrent map read and map write"
//
// or trigger the race detector. With the fix, the test should be stable.
// See: https://github.com/coredns/coredns/issues/7630
func TestIssue7630(t *testing.T) {
	name, rm, err := test.TempFile(".", exampleOrg)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	upstreamCorefile := `example.org:0 {
        file ` + name + `
    }`

	up, udpUp, _, err := CoreDNSServerAndPorts(upstreamCorefile)
	if err != nil {
		t.Fatalf("Could not start upstream CoreDNS: %s", err)
	}
	defer up.Stop()

	// Build an intentionally heavy log format that repeatedly reads the same metadata
	// to widen the read window.
	const repeat = 100
	logMetaReads := strings.Repeat("{/forward/upstream}", repeat)

	// Caching/forwarding server under test. Use 1s TTL and min TTL 0 to expire quickly
	// for both positive and negative responses.
	// Enable serve_stale so that every post-expiration query spawns a prefetch goroutine.
	// Include metadata and log that reads {/forward/upstream} set by forward.
	underTestCorefile := `example.org:0 {
        metadata
        log . "` + logMetaReads + `"
        forward . ` + udpUp + ` {
            health_check 5s domain example.org
        }
        cache {
            success 1000 1 0
            denial 1000 1 0
            serve_stale
        }
    }`

	inst, udp, _, err := CoreDNSServerAndPorts(underTestCorefile)
	if err != nil {
		t.Fatalf("Could not start test CoreDNS: %s", err)
	}
	defer inst.Stop()

	// Prime cache with some initial lookups to populate both positive and negative caches.
	m := new(dns.Msg)
	m.SetQuestion("short.example.org.", dns.TypeA)
	if _, err := dns.Exchange(m, udp); err != nil {
		t.Fatalf("priming positive query failed: %s", err)
	}
	// A few NXDOMAINs
	for i := range 50 {
		m := new(dns.Msg)
		m.SetQuestion(fmt.Sprintf("nx%d.example.org.", i), dns.TypeA)
		if _, err := dns.Exchange(m, udp); err != nil {
			t.Fatalf("priming negative query failed: %s", err)
		}
	}

	// Wait for TTL (1s) to expire so subsequent queries serve stale and trigger prefetch.
	time.Sleep(1300 * time.Millisecond)

	const (
		concurrency = 10
		iterations  = 100
		names       = 256
	)

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for range concurrency {
		go func() {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			for i := range iterations {
				// Mix of positive and negative queries to trigger many independent prefetches.
				var qname string
				if i%4 == 0 {
					qname = "short.example.org."
				} else {
					qname = fmt.Sprintf("nx%d.example.org.", r.Intn(names))
				}
				m := new(dns.Msg)
				m.SetQuestion(qname, dns.TypeA)
				if _, err := dns.Exchange(m, udp); err != nil {
					// Any error here is unexpected; if the historical race regresses,
					// the process may crash with concurrent map read/write before we see this.
					t.Errorf("query failed: %v", err)
					return
				}
				// Small delay to allow background prefetch to overlap with logging of other requests.
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}
	wg.Wait()
}
