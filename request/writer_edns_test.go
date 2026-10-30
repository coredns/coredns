package request

import (
	"fmt"
	"net"
	"reflect"
	"strings"
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

func TestScrubWriterEDNSBudget(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options []dns.EDNS0
		answers int
	}{
		{"large_ede", []dns.EDNS0{&dns.EDNS0_EDE{InfoCode: dns.ExtendedErrorCodeStaleAnswer, ExtraText: strings.Repeat("e", 700)}}, 1},
		{"ede_displaces_answers", []dns.EDNS0{&dns.EDNS0_EDE{InfoCode: dns.ExtendedErrorCodeStaleAnswer, ExtraText: strings.Repeat("e", 300)}}, 15},
		{"large_nsid", []dns.EDNS0{&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: strings.Repeat("ab", 700)}}, 1},
		{"large_local", []dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xffee, Data: []byte(strings.Repeat("l", 700))}}, 1},
		{"cookie_after_local", []dns.EDNS0{
			&dns.EDNS0_LOCAL{Code: 0xffef, Data: []byte(strings.Repeat("l", 460))},
			&dns.EDNS0_COOKIE{Code: dns.EDNS0COOKIE, Cookie: "0123456789abcdef0123456789abcdef"},
		}, 1},
		{"ecs_after_local", []dns.EDNS0{
			&dns.EDNS0_LOCAL{Code: 0xffef, Data: []byte(strings.Repeat("l", 460))},
			&dns.EDNS0_SUBNET{Code: dns.EDNS0SUBNET, Family: 2, SourceNetmask: 128, SourceScope: 128, Address: net.ParseIP("2001:db8::1")},
		}, 1},
		{"cookie_ecs_after_local", []dns.EDNS0{
			&dns.EDNS0_LOCAL{Code: 0xffef, Data: []byte(strings.Repeat("l", 430))},
			&dns.EDNS0_COOKIE{Code: dns.EDNS0COOKIE, Cookie: "0123456789abcdef0123456789abcdef"},
			&dns.EDNS0_SUBNET{Code: dns.EDNS0SUBNET, Family: 2, SourceNetmask: 128, SourceScope: 128, Address: net.ParseIP("2001:db8::1")},
		}, 1},
		{"small_ede", []dns.EDNS0{&dns.EDNS0_EDE{InfoCode: dns.ExtendedErrorCodeStaleAnswer, ExtraText: "stale"}}, 1},
		{"mixed", []dns.EDNS0{
			&dns.EDNS0_COOKIE{Code: dns.EDNS0COOKIE, Cookie: "0123456789abcdef0123456789abcdef"},
			&dns.EDNS0_EDE{InfoCode: dns.ExtendedErrorCodeStaleAnswer, ExtraText: strings.Repeat("e", 700)},
			&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: "c0ffee"},
		}, 1},
		{"extended_rcode", []dns.EDNS0{
			&dns.EDNS0_EDE{InfoCode: dns.ExtendedErrorCodeOther, ExtraText: strings.Repeat("e", 700)},
			&dns.EDNS0_COOKIE{Code: dns.EDNS0COOKIE, Cookie: "0123456789abcdef0123456789abcdef"},
		}, 1},
	} {
		for _, tcp := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tcp_%t", tc.name, tcp), func(t *testing.T) {
				req := new(dns.Msg).SetQuestion("example.com.", dns.TypeA).SetEdns0(512, true)
				for _, option := range tc.options {
					if option.Option() == dns.EDNS0COOKIE || option.Option() == dns.EDNS0SUBNET {
						req.IsEdns0().Option = append(req.IsEdns0().Option, option)
					}
				}
				mock := &mockResponseWriter{ResponseWriter: test.ResponseWriter{TCP: tcp}}
				resp := new(dns.Msg).SetReply(req).SetEdns0(4096, false)
				if tc.name == "extended_rcode" {
					resp.Rcode = dns.RcodeBadCookie
				}
				for range tc.answers {
					resp.Answer = append(resp.Answer, test.A("example.com. 300 IN A 192.0.2.1"))
				}
				resp.IsEdns0().Option = tc.options
				original := dns.Copy(resp.IsEdns0()).(*dns.OPT)
				if err := NewScrubWriter(req, mock).WriteMsg(resp); err != nil {
					t.Fatal(err)
				}
				wire, err := mock.lastMsg.Pack()
				if err != nil {
					t.Fatal(err)
				}
				decoded := new(dns.Msg)
				if err := decoded.Unpack(wire); err != nil || decoded.Rcode != resp.Rcode {
					t.Fatalf("response RCODE did not survive encoding: %d, %v", decoded.Rcode, err)
				}
				opt := mock.lastMsg.IsEdns0()
				if opt == nil || !opt.Do() || opt.UDPSize() != 512 {
					t.Fatalf("client EDNS header was lost: %v", opt)
				}
				if len(mock.lastMsg.Answer) != tc.answers {
					t.Fatalf("answers = %d, want %d", len(mock.lastMsg.Answer), tc.answers)
				}
				if tcp || tc.name == "small_ede" {
					if mock.lastMsg.Truncated || !reflect.DeepEqual(opt.Option, original.Option) {
						t.Fatalf("fitting response metadata was changed: %s", mock.lastMsg)
					}
				} else {
					if len(wire) > 512 || !mock.lastMsg.Truncated {
						t.Fatalf("UDP response size = %d, TC = %t", len(wire), mock.lastMsg.Truncated)
					}
					if strings.HasSuffix(tc.name, "_after_local") && !reflect.DeepEqual(opt.Option, tc.options[1:]) {
						t.Errorf("required response option was lost: %v", opt.Option)
					}
					if tc.name == "mixed" && !reflect.DeepEqual(opt.Option, []dns.EDNS0{tc.options[0], tc.options[2]}) {
						t.Errorf("other response options were lost: %v", opt.Option)
					}
				}
				if !reflect.DeepEqual(tc.options, original.Option) {
					t.Error("scrubbing changed the source options")
				}
			})
		}
	}
}
