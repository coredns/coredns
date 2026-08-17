package rewrite

import (
	"bytes"
	"context"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// serveEdns0Rewrite runs req through a rewrite plugin holding rule, with next as
// the upstream handler, wrapping the client writer in a ScrubWriter exactly as
// the server does. It returns the message delivered to the client.
func serveEdns0Rewrite(t *testing.T, rule Rule, next plugin.Handler, req *dns.Msg) *dns.Msg {
	t.Helper()
	rw := Rewrite{
		Next:         next,
		Rules:        []Rule{rule},
		RevertPolicy: NewRevertPolicy(false, false),
	}
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	// The server wraps the client writer in a ScrubWriter; reproduce that here.
	sw := request.NewScrubWriter(req, rec)
	rcode, err := rw.ServeDNS(context.Background(), sw, req)
	if err != nil {
		t.Fatal(err)
	}
	if !plugin.ClientWrite(rcode) {
		state := request.Request{W: sw, Req: req}
		resp := new(dns.Msg).SetRcode(req, rcode)
		state.SizeAndDo(resp)
		if err := sw.WriteMsg(resp); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Msg
}

// noOptReply returns a handler whose upstream reply carries the given answers
// and, crucially, no OPT record — so ScrubWriter falls back to the request OPT.
func noOptReply(answer ...dns.RR) plugin.Handler {
	return plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		resp := new(dns.Msg)
		resp.SetReply(r)
		resp.Answer = answer
		if err := w.WriteMsg(resp); err != nil {
			return dns.RcodeServerFailure, err
		}
		return dns.RcodeSuccess, nil
	})
}

func localOptionData(msg *dns.Msg, code uint16) ([]byte, bool) {
	opt := msg.IsEdns0()
	if opt == nil {
		return nil, false
	}
	for _, o := range opt.Option {
		if l, ok := o.(*dns.EDNS0_LOCAL); ok && l.Code == code {
			return l.Data, true
		}
	}
	return nil, false
}

// TestEdns0LocalSetRevertNoLeak covers #8234 for an answer-bearing reply.
//
// "rewrite edns0 local set <code> <data> revert" adds the option to the request
// OPT for the upstream only. When the upstream reply carries no OPT of its own,
// ScrubWriter.SizeAndDo gives the reply an OPT by reusing the request's, so the
// injected option must not be visible on the request while the reply is written.
// TestRewriteEDNS0RevertDoesNotLeakThroughScrubWriter covers a reply with no
// records at all; this one drives the same path with a record present, so the
// per-record response rules run as well.
func TestEdns0LocalSetRevertNoLeak(t *testing.T) {
	const code = 0xffee

	rule, err := newEdns0Rule("stop", "local", "set", "0xffee", "0xabcdef", "revert")
	if err != nil {
		t.Fatal(err)
	}

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	m.SetEdns0(4096, false)

	got := serveEdns0Rewrite(t, rule, noOptReply(test.A("example.org. 3600 IN A 127.0.0.1")), m)

	if _, ok := localOptionData(got, code); ok {
		t.Fatalf("client reply leaked EDNS0 local option 0x%x; revert must strip it (#8234)", code)
	}
}

// TestEdns0LocalSetRevertRestoresPreexisting covers the replace path of #8234:
// when the client request already carries the option, "set ... revert" overwrites
// it for the upstream, so the reply must show the client's original value again —
// not merely have the option removed — since ScrubWriter reuses the request OPT
// for an OPT-less reply.
func TestEdns0LocalSetRevertRestoresPreexisting(t *testing.T) {
	const code = 0xffee
	original := []byte{0x11, 0x11, 0x11}

	rule, err := newEdns0Rule("stop", "local", "set", "0xffee", "0xabcdef", "revert")
	if err != nil {
		t.Fatal(err)
	}

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	o := &dns.OPT{Hdr: dns.RR_Header{Name: ".", Rrtype: dns.TypeOPT}}
	o.SetUDPSize(4096)
	o.Option = append(o.Option, &dns.EDNS0_LOCAL{Code: code, Data: original})
	m.Extra = append(m.Extra, o)

	got := serveEdns0Rewrite(t, rule, noOptReply(test.A("example.org. 3600 IN A 127.0.0.1")), m)

	data, ok := localOptionData(got, code)
	if !ok {
		t.Fatalf("client reply dropped pre-existing EDNS0 local option 0x%x", code)
	}
	if !bytes.Equal(data, original) {
		t.Fatalf("revert must restore the original option value: got 0x%x, want 0x%x (#8234)", data, original)
	}
}
