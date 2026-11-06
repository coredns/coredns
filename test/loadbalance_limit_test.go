package test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestLoadbalanceLimitAnswers(t *testing.T) {
	dir := t.TempDir()
	zone := filepath.ToSlash(filepath.Join(dir, "db.example"))
	weights := filepath.ToSlash(filepath.Join(dir, "weights"))
	if err := os.WriteFile(zone, []byte(`$ORIGIN example.
@ 60 IN SOA ns.example. hostmaster.example. 1 60 60 60 60
@ 60 IN NS ns.example.
ns 60 IN A 192.0.2.53
alias 60 IN CNAME middle.example.
middle 60 IN CNAME target.example.
target 60 IN A 192.0.2.1
target 60 IN A 198.51.100.1
target 60 IN A 192.0.2.2
target 60 IN AAAA 2001:db8::1
target 60 IN AAAA 2001:db8:1::1
target 60 IN AAAA 2001:db8::2
`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(weights, []byte("target.example.\n192.0.2.1 255\n198.51.100.1 1\n2001:db8::1 255\n2001:db8:1::1 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, config string
		limit        int
		preferred    bool
	}{
		{"default", "loadbalance", 3, false},
		{"round robin", "loadbalance {\nlimit_answers 2\n}", 2, false},
		{"weighted preferred", fmt.Sprintf("loadbalance weighted %q {\nprefer 198.51.100.0/24 2001:db8:1::/48\nreload 0s\nlimit_answers 1\n}", weights), 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			instance, udp, tcp, err := CoreDNSServerAndPorts(fmt.Sprintf(`example.:0 {
    bind 127.0.0.1
    %s
    cache 60
    file %q example. {
        reload 0
    }
}`, tc.config, zone))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { CoreDNSServerStop(instance) })
			for _, network := range []string{"udp", "tcp"} {
				for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
					t.Run(network+"/"+dns.TypeToString[qtype], func(t *testing.T) {
						t.Parallel()
						addr := udp
						if network == "tcp" {
							addr = tcp
						}
						client := &dns.Client{Net: network, Timeout: 5 * time.Second}
						for range 10 {
							for _, do := range []bool{false, true} {
								q := new(dns.Msg)
								q.SetQuestion("alias.example.", qtype)
								q.SetEdns0(1232, do)
								response, _, err := client.Exchange(q, addr)
								if err != nil {
									t.Fatal(err)
								}
								count := tc.limit
								if do {
									count = 3
								}
								if response.Rcode != dns.RcodeSuccess || response.Truncated || len(response.Answer) != count+2 {
									t.Fatalf("DO=%v: unexpected answer: %s", do, response)
								}
								for i, target := range []string{"middle.example.", "target.example."} {
									cname, ok := response.Answer[i].(*dns.CNAME)
									if !ok || cname.Target != target {
										t.Fatalf("broken alias chain: %s", response)
									}
								}
								if tc.preferred && !do {
									var ip string
									switch rr := response.Answer[2].(type) {
									case *dns.A:
										ip = rr.A.String()
									case *dns.AAAA:
										ip = rr.AAAA.String()
									}
									if ip != "198.51.100.1" && ip != "2001:db8:1::1" {
										t.Fatalf("lost preferred address: %s", response)
									}
								}
							}
						}
					})
				}
			}
		})
	}
}
