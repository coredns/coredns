package test

import (
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestMinimalRewriteEDNS(t *testing.T) {
	options := []dns.EDNS0{
		&dns.EDNS0_EDE{InfoCode: dns.ExtendedErrorCodeStaleAnswer, ExtraText: "stale answer"},
		&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: "c0ffee"},
		&dns.EDNS0_COOKIE{Code: dns.EDNS0COOKIE, Cookie: "0123456789abcdef0123456789abcdef"},
	}
	local := &dns.EDNS0_LOCAL{Code: 0xffee, Data: []byte{0xab, 0xcd, 0xef}}

	for _, downstreamDo := range []bool{false, true} {
		t.Run(fmt.Sprintf("downstream_do_%t", downstreamDo), func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { listener.Close() })
			packet, err := net.ListenPacket("udp", listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { packet.Close() })

			requests := make(chan *dns.Msg, 1)
			handler := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
				requests <- r.Copy()
				msg := new(dns.Msg).SetReply(r)
				msg.Answer = []dns.RR{test.A("example.com. 300 IN A 192.0.2.1")}
				msg.Ns = []dns.RR{test.NS("example.com. 300 IN NS ns.example.com.")}
				msg.Extra = []dns.RR{test.A("ns.example.com. 300 IN A 192.0.2.2")}
				msg.SetEdns0(4096, downstreamDo)
				msg.IsEdns0().Option = append(options, local)
				if err := w.WriteMsg(msg); err != nil {
					t.Errorf("upstream reply: %v", err)
				}
			})
			for _, server := range []*dns.Server{
				{Listener: listener, Handler: handler},
				{PacketConn: packet, Handler: handler},
			} {
				started := make(chan struct{})
				stopped := make(chan error, 1)
				server.NotifyStartedFunc = func() { close(started) }
				go func() { stopped <- server.ActivateAndServe() }()
				select {
				case <-started:
				case err := <-stopped:
					t.Fatalf("starting upstream: %v", err)
				}
				t.Cleanup(func() {
					if err := server.Shutdown(); err != nil {
						t.Errorf("stopping upstream: %v", err)
					}
					if err := <-stopped; err != nil {
						t.Errorf("upstream server: %v", err)
					}
				})
			}

			i, udp, tcp, err := CoreDNSServerAndPorts(fmt.Sprintf(`.:0 {
				bufsize 1232
				rewrite edns0 local set 0xffee 0xabcdef revert
				minimal
				forward . %s
			}`, listener.Addr()))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { i.Stop() })

			for _, client := range []struct {
				name string
				size uint16
				do   bool
			}{
				{name: "no_edns"},
				{name: "do_false", size: 1232},
				{name: "do_true", size: 1232, do: true},
				{name: "bufsize", size: 4096, do: true},
			} {
				for _, transport := range []struct{ network, address string }{
					{network: "udp", address: udp},
					{network: "tcp", address: tcp},
				} {
					t.Run(client.name+"/"+transport.network, func(t *testing.T) {
						req := new(dns.Msg).SetQuestion("example.com.", dns.TypeA)
						if client.size != 0 {
							req.SetEdns0(client.size, client.do)
							req.IsEdns0().Option = []dns.EDNS0{
								&dns.EDNS0_NSID{Code: dns.EDNS0NSID},
								&dns.EDNS0_COOKIE{Code: dns.EDNS0COOKIE, Cookie: "0123456789abcdef"},
							}
						}
						c := &dns.Client{Net: transport.network, Timeout: time.Second}
						resp, _, err := c.Exchange(req, transport.address)
						if err != nil {
							t.Fatal(err)
						}
						upstream := <-requests
						opt := upstream.IsEdns0()
						if opt == nil {
							t.Fatal("rewrite did not add an upstream OPT record")
						}
						found := false
						for _, option := range opt.Option {
							if reflect.DeepEqual(option, local) {
								found = true
							}
						}
						if !found {
							t.Fatalf("upstream options = %v, want rewrite option %v", opt.Option, local)
						}
						if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 || len(resp.Ns) != 0 {
							t.Fatalf("unexpected minimized response: %s", resp)
						}
						opt = resp.IsEdns0()
						if client.size == 0 {
							if len(resp.Extra) != 0 {
								t.Fatalf("non-EDNS response has additional records: %s", resp)
							}
							return
						}
						if opt == nil || len(resp.Extra) != 1 {
							t.Fatalf("expected only OPT in additional section: %s", resp)
						}
						if len(opt.Option) != len(options) {
							t.Fatalf("response options = %v, want %v", opt.Option, options)
						}
						for n, option := range options {
							got := opt.Option[n]
							if got.Option() != option.Option() || got.String() != option.String() {
								t.Errorf("response option %d = %#v, want %#v", n, opt.Option[n], option)
							}
						}
						if opt.UDPSize() != 1232 || opt.Do() != client.do {
							t.Errorf("response UDP size/DO = %d/%t, want 1232/%t", opt.UDPSize(), opt.Do(), client.do)
						}
					})
				}
			}
		})
	}
}

func TestMinimalEDNSBudget(t *testing.T) {
	for _, signed := range []bool{false, true} {
		for _, tc := range []struct {
			name    string
			option  dns.EDNS0
			answers int
		}{
			{"large_ede", &dns.EDNS0_EDE{InfoCode: dns.ExtendedErrorCodeStaleAnswer, ExtraText: strings.Repeat("e", 700)}, 1},
			{"ede_displaces_answers", &dns.EDNS0_EDE{InfoCode: dns.ExtendedErrorCodeStaleAnswer, ExtraText: strings.Repeat("e", 300)}, 15},
			{"large_nsid", &dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: strings.Repeat("ab", 700)}, 1},
			{"large_local", &dns.EDNS0_LOCAL{Code: 0xffef, Data: []byte(strings.Repeat("l", 700))}, 1},
			{"tsig_budget", &dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: strings.Repeat("ab", 340)}, 4},
			{"tsig_compression", &dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: strings.Repeat("ab", 300)}, 4},
		} {
			t.Run(fmt.Sprintf("%s/signed_%t", tc.name, signed), func(t *testing.T) {
				if signed && tc.name == "ede_displaces_answers" {
					tc.answers = 10
				}
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				started := make(chan struct{})
				stopped := make(chan error, 1)
				upstream := &dns.Server{Listener: listener, NotifyStartedFunc: func() { close(started) }, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
					msg := new(dns.Msg).SetReply(r).SetEdns0(4096, false)
					for range tc.answers {
						msg.Answer = append(msg.Answer, test.A("example.com. 300 IN A 192.0.2.1"))
					}
					msg.IsEdns0().Option = []dns.EDNS0{tc.option}
					if err := w.WriteMsg(msg); err != nil {
						t.Error(err)
					}
				})}
				go func() { stopped <- upstream.ActivateAndServe() }()
				<-started
				t.Cleanup(func() {
					upstream.Shutdown()
					if err := <-stopped; err != nil {
						t.Error(err)
					}
				})
				config := fmt.Sprintf(".:0 {\nbufsize 512\nrewrite edns0 local set 0xffee 0xabcdef revert\nminimal\nforward . %s {\nforce_tcp\n}\n", listener.Addr())
				if signed {
					config += "tsig {\nsecret " + tsigKey + " " + tsigSecret + "\n}\n"
				}
				instance, udp, tcp, err := CoreDNSServerAndPorts(config + "}")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { instance.Stop() })
				for _, transport := range []struct{ network, address string }{{"udp", udp}, {"tcp", tcp}} {
					t.Run(transport.network, func(t *testing.T) {
						req := new(dns.Msg).SetQuestion("example.com.", dns.TypeA).SetEdns0(4096, true)
						client := &dns.Client{Net: transport.network, Timeout: time.Second}
						if signed {
							req.SetTsig(tsigKey, dns.HmacSHA256, 300, time.Now().Unix())
							client.TsigSecret = map[string]string{tsigKey: tsigSecret}
						}
						conn, err := client.Dial(transport.address)
						if err != nil {
							t.Fatal(err)
						}
						defer conn.Close()
						conn.TsigSecret = client.TsigSecret
						conn.SetDeadline(time.Now().Add(time.Second))
						var requestMAC string
						if signed {
							var query []byte
							query, requestMAC, err = dns.TsigGenerate(req, tsigSecret, "", false)
							if err == nil {
								_, err = conn.Write(query)
							}
						} else {
							err = conn.WriteMsg(req)
						}
						if err != nil {
							t.Fatal(err)
						}
						// Read the actual wire bytes, preserving their compression and MAC.
						conn.UDPSize = dns.MaxMsgSize
						wire, err := conn.ReadMsgHeader(nil)
						if err != nil {
							t.Fatal(err)
						}
						resp := new(dns.Msg)
						if err := resp.Unpack(wire); err != nil {
							t.Fatal(err)
						}
						if signed {
							if resp.IsTsig() == nil {
								t.Fatal("TSIG missing or not last")
							}
							if err := dns.TsigVerify(wire, tsigSecret, requestMAC, false); err != nil {
								t.Fatalf("invalid TSIG: %v", err)
							}
						}
						opt := resp.IsEdns0()
						wantAnswers := tc.answers
						if tc.name == "tsig_budget" && signed && transport.network == "udp" {
							wantAnswers = 0
						}
						if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != wantAnswers || opt == nil || !opt.Do() || opt.UDPSize() != 512 {
							t.Fatalf("unexpected response: %s", resp)
						}
						if transport.network == "udp" {
							wantOptions := 0
							wantTC := true
							if strings.HasPrefix(tc.name, "tsig_") {
								wantOptions = 1
								wantTC = signed && tc.name == "tsig_budget"
							}
							if len(wire) > 512 || resp.Truncated != wantTC || len(opt.Option) != wantOptions {
								t.Fatalf("UDP size=%d TC=%t options=%v", len(wire), resp.Truncated, opt.Option)
							}
						} else if resp.Truncated || len(opt.Option) != 1 || opt.Option[0].String() != tc.option.String() {
							t.Fatal("TCP metadata changed")
						}
					})
				}
			})
		}
	}
}
