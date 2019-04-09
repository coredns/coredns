package hosts

// RLS - Wildcard matching and benchmarking
import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	"os"
	"bufio"
	"fmt"
	"time"
	"strconv"
	"sync"
)

func TestLookupLargeHosts(t *testing.T) {
	h := Hosts{Next: test.ErrorHandler(), Hostsfile: &Hostsfile{Origins: []string{"."}}}
	h.parseReader(strings.NewReader(largeHostsExample))

	ctx := context.TODO()

	for _, tc := range largeHostsTestCases {
		m := tc.Msg()

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := h.ServeDNS(ctx, rec, m)
		if err != nil {
			t.Errorf("Expected no error, got %v\n", err)
			return
		}

		//fmt.Println("[TRACE] rec:", rec)
		resp := rec.Msg
		test.SortAndCheck(t, resp, tc)
	}
}

// go test -run=nothing -bench=LargeHosts
func BenchmarkLookupLargeHosts(b *testing.B) {
	filename := "./largehost.txt"
	fp, err := os.Open(filename)
	if err != nil {
		fmt.Printf("Couldn't open file. %v\n", err)
		os.Exit(1)
	}
	defer fp.Close()

	h := Hosts{Next: test.ErrorHandler(), Hostsfile: &Hostsfile{Origins: []string{"."}}}
	h.parseReader(bufio.NewReader(fp))

	b.ResetTimer()

	var hosts = []string{"m.trb.com", "ntvcld-a.akamaihd.net", "chirecommend.troncdata.com", "ssor.tribdss.com", "secure.footprint.net", "ads.twitter.com", "b.scorecardresearch.com", "ads.rubiconproject.com"}
	var lookupsperthread = 20
	var wg sync.WaitGroup
	for ind := 0; ind < len(hosts); ind++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var totaltime time.Duration
			for count := 0; count < lookupsperthread; count++ {
				host := "subdomain" + strconv.Itoa(count) + "." + hosts[i]
				start := time.Now()

				ctx := context.TODO()
				m := msg(host)

				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				_, err := h.ServeDNS(ctx, rec, m)
				if err != nil {
					fmt.Printf("Expected no error, got %v\n", err)
					return
				}

				elapsed := time.Since(start)
				if elapsed > time.Duration(2) * time.Second {
					fmt.Println("[ERROR] DNS Lookup too slow", host, elapsed)
				}

				time.Sleep(10 * time.Millisecond)
			}
			fmt.Println("[TEST] Average time per lookup", totaltime / time.Duration(lookupsperthread))
		}(ind)
	}
	wg.Wait()
	fmt.Println("[TEST] Benchmark complete.")
}

func msg(Qname string) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(Qname), dns.TypeA)
	//if c.Do {
	//	o := new(dns.OPT)
	//	o.Hdr.Name = "."
	//	o.Hdr.Rrtype = dns.TypeOPT
	//	o.SetDo()
	//	o.SetUDPSize(4096)
	//	m.Extra = []dns.RR{o}
	//}
	return m
}

var largeHostsTestCases = []test.Case{
	{
		Qname: "ezula.com.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("ezula.com. 3600	IN	A 192.168.102.1"),
		},
	},
	{
		Qname: "abc.def.ezula.com.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("abc.def.ezula.com. 3600	IN	A 192.168.102.1"),
		},
	},
}

const largeHostsExample = `
127.0.0.1 localhost localhost.domain
::1 localhost localhost.domain
192.168.102.1 ezula.com`
