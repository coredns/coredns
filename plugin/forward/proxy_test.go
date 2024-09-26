package forward

import (
	"context"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// I want to add tests here to confirm:
//  1. We don't relay client EDNS to upstream (Check)
//  2. We don't copy client EDNS back on the response (todo)
func TestProxy(t *testing.T) {
	testCases := []struct {
		name         string
		query        test.Case // the client query
		upstreamResp test.Case // the upstream response to the forwarded query
		expectedResp test.Case // the expected upstream response
	}{
		{
			name: "simple A query",
			query: test.Case{
				Qname: "example.com.", Qtype: dns.TypeA,
			},
			upstreamResp: test.Case{
				Answer: []dns.RR{
					test.A("example.org. IN A 127.0.0.1"),
				},
			},
			expectedResp: test.Case{
				Answer: []dns.RR{
					test.A("example.org. IN A 127.0.0.1"),
				},
			},
		},
		{
			name: "query EDNS opts shouldn't get copied back to response",
			query: test.Case{
				Qname: "example.com.", Qtype: dns.TypeA,
				Extra: []dns.RR{
					test.OPT(1232, true, &dns.EDNS0_NSID{Nsid: "NSID"}, &dns.EDNS0_EXPIRE{Expire: 1234567}),
				},
			},
			upstreamResp: test.Case{
				Answer: []dns.RR{
					test.A("example.org. IN A 127.0.0.1"),
				},
			},
			expectedResp: test.Case{
				Answer: []dns.RR{
					test.A("example.org. IN A 127.0.0.1"),
				},
			},
		},
		{
			// TODO: This test case fails because the forward plugin IS copying back EDNS opts to the client...
			//       What does BIND DNS server do? Maybe this is acceptable.
			name: "upstream server EDNS opts on response shouldn't get returned to client",
			query: test.Case{
				Qname: "example.com.", Qtype: dns.TypeA,
			},
			upstreamResp: test.Case{
				Answer: []dns.RR{
					test.A("example.org. IN A 127.0.0.1"),
				},
				Extra: []dns.RR{
					test.OPT(1232, true, &dns.EDNS0_NSID{}),
				},
			},
			expectedResp: test.Case{
				Answer: []dns.RR{
					test.A("example.org. IN A 127.0.0.1"),
				},
			},
		},
		{
			// TODO: This fails for a really odd reason, the forward plugin times out...
			name: "upstream server EDNS opts on response shouldn't get returned to client",
			query: test.Case{
				Qname: "example.com.", Qtype: dns.TypeA,
			},
			upstreamResp: test.Case{
				Answer: []dns.RR{
					test.A("example.org. IN A 127.0.0.1"),
				},
				Extra: []dns.RR{
					test.OPT(1232, true, &dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: "NSID"}),
				},
			},
			expectedResp: test.Case{
				Answer: []dns.RR{
					test.A("example.org. IN A 127.0.0.1"),
				},
			},
		},
		{
			name: "forwarding shouldn't modify header values",
			query: test.Case{
				Qname: "example.com.", Qtype: dns.TypeA,
			},
			upstreamResp: test.Case{
				Answer: []dns.RR{
					test.A("example.org. IN A 127.0.0.1"),
				},
				RecursionAvailable: true,
				AuthenticatedData:  true,
				Authoritative:      true,
				CheckingDisabled:   true,
				Truncated:          true,
				Do:                 true, // TODO: I don't know if DO bit test logic works...
			},
			expectedResp: test.Case{
				Answer: []dns.RR{
					test.A("example.org. IN A 127.0.0.1"),
				},
				RecursionAvailable: true,
				AuthenticatedData:  true,
				Authoritative:      true,
				CheckingDisabled:   true,
				Truncated:          true,
				Do:                 true,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.query.Msg()
			m = forwardMsg(m, tc.query)
			o := m.IsEdns0()
			queryEDNS := o != nil
			queryDoBit := o != nil && o.Do()

			s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
				// Run checks on forwarded query
				if o := r.IsEdns0(); o != nil {
					if !queryEDNS {
						t.Fatal("Expected forwarded query NOT to have EDNS since the query didnt have EDNS")
					}
					if queryDoBit != o.Do() {
						t.Fatalf("Expected query DO Bit (%t) was expected to match forwarded query DO bit (%t)", queryDoBit, o.Do())
					}
					if len(o.Option) != 0 {
						t.Fatalf("Expected 0 EDNS options, but got %d", o.Option)
					}
				} else if queryEDNS {
					t.Fatal("Expected forwarded query to have EDNS since the query had EDNS")
				}
				ret := tc.upstreamResp.Msg()
				ret.SetReply(r)
				ret = forwardMsg(ret, tc.upstreamResp)
				w.WriteMsg(ret)
			})
			defer s.Close()

			c := caddy.NewTestController("dns", "forward . "+s.Addr)
			fs, err := parseForward(c)
			f := fs[0]
			if err != nil {
				t.Errorf("Failed to create forwarder: %s", err)
			}
			f.OnStartup()
			defer f.OnShutdown()

			resp := dnstest.NewRecorder(&test.ResponseWriter{})

			if _, err := f.ServeDNS(context.TODO(), resp, m); err != nil {
				t.Fatal("Expected to receive reply, but didn't")
			}
			if err := test.Header(tc.expectedResp, resp.Msg); err != nil {
				t.Error(err)
			}
			if err := test.Section(tc.expectedResp, test.Answer, resp.Msg.Answer); err != nil {
				t.Error(err)
			}
			if err := test.Section(tc.expectedResp, test.Ns, resp.Msg.Ns); err != nil {
				t.Error(err)
			}
			if err := test.Section(tc.expectedResp, test.Extra, resp.Msg.Extra); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestProxyTLSFail(t *testing.T) {
	// This is an udp/tcp test server, so we shouldn't reach it with TLS.
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
		w.WriteMsg(ret)
	})
	defer s.Close()

	c := caddy.NewTestController("dns", "forward . tls://"+s.Addr)
	fs, err := parseForward(c)
	f := fs[0]
	if err != nil {
		t.Errorf("Failed to create forwarder: %s", err)
	}
	f.OnStartup()
	defer f.OnShutdown()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err == nil {
		t.Fatal("Expected *not* to receive reply, but got one")
	}
}

func forwardMsg(m *dns.Msg, tc test.Case) *dns.Msg {
	m.RecursionAvailable = tc.RecursionAvailable
	m.AuthenticatedData = tc.AuthenticatedData
	m.CheckingDisabled = tc.CheckingDisabled
	m.Authoritative = tc.Authoritative
	m.Rcode = tc.Rcode
	m.Truncated = tc.Truncated
	m.Answer = tc.Answer
	m.Ns = tc.Ns
	m.Extra = tc.Extra
	return m
}
