package request

import (
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestScrubWriterEDNSRequestMutation(t *testing.T) {
	for _, client := range []struct {
		name string
		edns bool
		do   bool
	}{
		{name: "no_edns"},
		{name: "do_false", edns: true},
		{name: "do_true", edns: true, do: true},
	} {
		for _, responseEDNS := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/response_edns_%t", client.name, responseEDNS), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetQuestion("example.com.", dns.TypeA)
				if client.edns {
					req.SetEdns0(4096, client.do)
				}
				mock := &mockResponseWriter{}
				sw := NewScrubWriter(req, mock)

				// Plugins can add EDNS internally, change DO, and clamp the buffer size.
				if req.IsEdns0() == nil {
					req.SetEdns0(4096, true)
				}
				req.IsEdns0().SetDo(!client.do)
				req.IsEdns0().SetUDPSize(1232)
				resp := new(dns.Msg).SetReply(req)
				resp.Answer = []dns.RR{test.A("example.com. 300 IN A 192.0.2.1")}
				if responseEDNS {
					resp.SetEdns0(4096, !client.do)
					resp.IsEdns0().Option = []dns.EDNS0{
						&dns.EDNS0_EDE{InfoCode: dns.ExtendedErrorCodeStaleAnswer},
					}
				}
				if err := sw.WriteMsg(resp); err != nil {
					t.Fatal(err)
				}
				opt := mock.lastMsg.IsEdns0()
				if !client.edns {
					if opt != nil {
						t.Fatalf("non-EDNS client received an OPT: %s", mock.lastMsg)
					}
					return
				}
				if opt == nil {
					t.Fatal("EDNS client received no OPT")
				}
				if opt.Do() != client.do || opt.UDPSize() != 1232 {
					t.Errorf("response DO/size = %t/%d, want %t/1232", opt.Do(), opt.UDPSize(), client.do)
				}
				if responseEDNS && len(opt.Option) != 1 {
					t.Errorf("response EDNS options lost: %v", opt.Option)
				}
			})
		}
	}
}

func TestScrubWriterNonEDNSBufferAfterRewrite(t *testing.T) {
	for _, tcp := range []bool{false, true} {
		t.Run(fmt.Sprintf("tcp_%t", tcp), func(t *testing.T) {
			req := new(dns.Msg).SetQuestion("example.com.", dns.TypeA)
			mock := &mockResponseWriter{ResponseWriter: test.ResponseWriter{TCP: tcp}}
			sw := NewScrubWriter(req, mock)
			req.SetEdns0(4096, false)
			resp := new(dns.Msg).SetReply(req)
			for range 100 {
				resp.Answer = append(resp.Answer, test.A("example.com. 300 IN A 192.0.2.1"))
			}
			if err := sw.WriteMsg(resp); err != nil {
				t.Fatal(err)
			}
			if tcp {
				if mock.lastMsg.Truncated || len(mock.lastMsg.Answer) != 100 {
					t.Errorf("TCP response was truncated: %s", mock.lastMsg)
				}
			} else if !mock.lastMsg.Truncated || mock.lastMsg.Len() > dns.MinMsgSize {
				t.Errorf("response exceeds non-EDNS UDP buffer: truncated=%t, size=%d", mock.lastMsg.Truncated, mock.lastMsg.Len())
			}
			if mock.lastMsg.IsEdns0() != nil {
				t.Error("non-EDNS response has an OPT")
			}
		})
	}
}

func TestScrubWriterExtendedRcode(t *testing.T) {
	for _, edns := range []bool{false, true} {
		for _, rcode := range []int{dns.RcodeBadVers, dns.RcodeBadCookie} {
			t.Run(fmt.Sprintf("edns_%t/%s", edns, dns.RcodeToString[rcode]), func(t *testing.T) {
				req := new(dns.Msg).SetQuestion("example.com.", dns.TypeA)
				if edns {
					req.SetEdns0(1232, false)
				}
				mock := &mockResponseWriter{}
				sw := NewScrubWriter(req, mock)
				if !edns {
					req.SetEdns0(4096, false)
				}
				resp := new(dns.Msg).SetRcode(req, rcode).SetEdns0(4096, false)
				if err := sw.WriteMsg(resp); err != nil {
					t.Fatal(err)
				}
				want := dns.RcodeServerFailure
				if edns {
					want = rcode
				}
				if mock.lastMsg.Rcode != want {
					t.Errorf("rcode = %d, want %d", mock.lastMsg.Rcode, want)
				}
				if _, err := mock.lastMsg.Pack(); err != nil {
					t.Fatalf("response cannot be encoded: %v", err)
				}
			})
		}
	}
}
