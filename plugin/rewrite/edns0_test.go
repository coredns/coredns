package rewrite

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// TestEdns0LocalSetRevertNoLeak is a regression test for #8234.
//
// "rewrite edns0 local set <code> <data> revert" adds the option to the request
// (for the upstream) only. When the upstream reply carries no OPT record, the
// server's ScrubWriter.SizeAndDo copies the request's OPT — which still holds
// the injected option — into the reply sent to the client. The revert must
// therefore strip the option from the request as well, otherwise it leaks.
func TestEdns0LocalSetRevertNoLeak(t *testing.T) {
	const code = 0xffee

	rule, err := newEdns0Rule("stop", "local", "set", "0xffee", "0xabcdef", "revert")
	if err != nil {
		t.Fatal(err)
	}

	// Mimic a forwarder whose upstream reply has NO OPT record.
	next := plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		resp := new(dns.Msg)
		resp.SetReply(r)
		resp.Answer = []dns.RR{test.A("example.org. 3600 IN A 127.0.0.1")}
		if err := w.WriteMsg(resp); err != nil {
			return dns.RcodeServerFailure, err
		}
		return dns.RcodeSuccess, nil
	})

	rw := Rewrite{
		Next:         next,
		Rules:        []Rule{rule},
		RevertPolicy: NewRevertPolicy(false, false),
	}

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	m.SetEdns0(4096, false)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	// The server wraps the client writer in a ScrubWriter; reproduce that here.
	sw := request.NewScrubWriter(m, rec)

	if _, err := rw.ServeDNS(context.Background(), sw, m); err != nil {
		t.Fatal(err)
	}

	if opt := rec.Msg.IsEdns0(); opt != nil {
		for _, o := range opt.Option {
			if o.Option() == code {
				t.Fatalf("client reply leaked EDNS0 local option 0x%x; revert must strip it (#8234)", code)
			}
		}
	}
}
