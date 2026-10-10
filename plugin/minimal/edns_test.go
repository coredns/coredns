package minimal

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestMinimizeResponseEDNS(t *testing.T) {
	for _, do := range []bool{false, true} {
		for _, option := range []dns.EDNS0{
			&dns.EDNS0_EDE{InfoCode: dns.ExtendedErrorCodeStaleAnswer, ExtraText: "stale answer"},
			&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: "c0ffee"},
			&dns.EDNS0_COOKIE{Code: dns.EDNS0COOKIE, Cookie: "0123456789abcdef0123456789abcdef"},
		} {
			t.Run(fmt.Sprintf("option_%d/do_%t", option.Option(), do), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetQuestion("example.com.", dns.TypeA)
				req.SetEdns0(1232, do)
				req.IsEdns0().Option = []dns.EDNS0{
					&dns.EDNS0_NSID{Code: dns.EDNS0NSID},
					&dns.EDNS0_COOKIE{Code: dns.EDNS0COOKIE, Cookie: "0123456789abcdef"},
				}

				downstream := plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
					msg := new(dns.Msg)
					msg.SetReply(r)
					msg.Answer = []dns.RR{test.A("example.com. 300 IN A 192.0.2.1")}
					msg.Ns = []dns.RR{test.NS("example.com. 300 IN NS ns.example.com.")}
					msg.Extra = []dns.RR{test.A("ns.example.com. 300 IN A 192.0.2.2")}
					msg.SetEdns0(4096, false)
					msg.IsEdns0().Option = []dns.EDNS0{option}
					return dns.RcodeSuccess, w.WriteMsg(msg)
				})

				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				h := minimalHandler{Next: downstream}
				_, err := h.ServeDNS(t.Context(), request.NewScrubWriter(req, rec), req)
				if err != nil {
					t.Fatal(err)
				}
				if len(rec.Msg.Answer) != 1 || len(rec.Msg.Ns) != 0 || len(rec.Msg.Extra) != 1 {
					t.Fatalf("unexpected minimized sections: %s", rec.Msg)
				}
				opt := rec.Msg.IsEdns0()
				if opt == nil {
					t.Fatal("response has no OPT record")
				}
				if !reflect.DeepEqual(opt.Option, []dns.EDNS0{option}) {
					t.Errorf("response options = %v, want %v", opt.Option, option)
				}
				if opt.UDPSize() != 1232 || opt.Do() != do {
					t.Errorf("response UDP size/DO = %d/%t, want 1232/%t", opt.UDPSize(), opt.Do(), do)
				}
			})
		}
	}
}
