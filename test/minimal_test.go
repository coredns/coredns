package test

import (
	"fmt"
	"net"
	"reflect"
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
