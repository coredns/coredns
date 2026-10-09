package test

import (
	"fmt"
	"testing"

	"github.com/miekg/dns"
)

func TestDNS64FallbackError(t *testing.T) {
	for _, rcode := range []int{dns.RcodeServerFailure, dns.RcodeRefused, dns.RcodeNameError, dns.RcodeNotImplemented} {
		t.Run(dns.RcodeToString[rcode], func(t *testing.T) {
			corefile := fmt.Sprintf(`.:0 {
				bind 127.0.0.1
				dns64 {
					allow_ipv4
				}
				template IN AAAA example.org {
					rcode NOERROR
				}
				template IN A example.org {
					rcode %s
				}
			}`, dns.RcodeToString[rcode])
			i, udp, tcp, err := CoreDNSServerAndPorts(corefile)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { CoreDNSServerStop(i) })

			for _, transport := range []struct {
				net  string
				addr string
			}{{"udp", udp}, {"tcp", tcp}} {
				t.Run(transport.net, func(t *testing.T) {
					query := new(dns.Msg)
					query.SetQuestion("example.org.", dns.TypeAAAA)
					client := &dns.Client{Net: transport.net}
					response, _, err := client.Exchange(query, transport.addr)
					if err != nil {
						t.Fatal(err)
					}
					if response.Rcode != rcode {
						t.Errorf("A fallback %s returned %s to AAAA client", dns.RcodeToString[rcode], dns.RcodeToString[response.Rcode])
					}
					if len(response.Question) != 1 || response.Question[0] != query.Question[0] {
						t.Errorf("response did not preserve the AAAA question: %s", response)
					}
				})
			}
		})
	}
}
