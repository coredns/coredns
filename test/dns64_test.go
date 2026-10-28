package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestDNS64FallbackError(t *testing.T) {
	for _, rcode := range []int{dns.RcodeSuccess, dns.RcodeServerFailure, dns.RcodeRefused, dns.RcodeFormatError, dns.RcodeNameError, dns.RcodeNotImplemented} {
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
				template IN TXT example.org {
					answer "{{ .Name }} 60 IN TXT \"connection reused\""
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
					client := &dns.Client{Net: transport.net}
					conn, err := client.Dial(transport.addr)
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()

					for attempt := range 3 {
						for _, tc := range []struct {
							qtype uint16
							id    uint16
							code  int
						}{
							{dns.TypeAAAA, uint16(101 + attempt), rcode},
							{dns.TypeTXT, uint16(202 + attempt), dns.RcodeSuccess},
						} {
							query := new(dns.Msg)
							query.SetQuestion("example.org.", tc.qtype)
							query.Id = tc.id
							if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
								t.Fatal(err)
							}
							if err := conn.WriteMsg(query); err != nil {
								t.Fatal(err)
							}
							response, err := conn.ReadMsg()
							if err != nil {
								t.Fatal(err)
							}
							if response.Id != query.Id || len(response.Question) != 1 || response.Question[0] != query.Question[0] {
								t.Fatalf("query ID %d (%s) received an unexpected reply: %s", query.Id, dns.TypeToString[tc.qtype], response)
							}
							if response.Rcode != tc.code {
								t.Errorf("query %s returned %s, want %s", dns.TypeToString[tc.qtype], dns.RcodeToString[response.Rcode], dns.RcodeToString[tc.code])
							}
							if tc.qtype == dns.TypeTXT {
								if len(response.Answer) != 1 || response.Answer[0].String() != "example.org.\t60\tIN\tTXT\t\"connection reused\"" {
									t.Errorf("connection reuse did not return the TXT answer: %s", response)
								}
							}
						}
					}
				})
			}
		})
	}
}
