package dnsserver

import (
	"context"
	"testing"

	"github.com/miekg/dns"
)

type tsigStatusCheckPlugin struct {
	t      *testing.T
	check  func(*testing.T, error)
	called chan struct{}
}

func (p tsigStatusCheckPlugin) Name() string { return "tsig-status-check" }

func (p tsigStatusCheckPlugin) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	p.t.Helper()
	if p.called != nil {
		p.called <- struct{}{}
	}
	p.check(p.t, w.TsigStatus())

	m := new(dns.Msg)
	m.SetReply(r)
	if err := w.WriteMsg(m); err != nil {
		p.t.Fatalf("WriteMsg() failed: %v", err)
	}
	return dns.RcodeSuccess, nil
}

func mustPackSignedTSIGQuery(t *testing.T, keyName, secret string, tsigTime int64) []byte {
	t.Helper()

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	m.Id = 0
	m.SetTsig(keyName, dns.HmacSHA256, 300, tsigTime)

	wire, _, err := dns.TsigGenerate(m, secret, "", false)
	if err != nil {
		t.Fatalf("dns.TsigGenerate() failed: %v", err)
	}
	return wire
}
