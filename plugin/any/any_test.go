package any

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestAny(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeANY)
	a := &Any{}

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	_, err := a.ServeDNS(context.TODO(), rec, req)

	if err != nil {
		t.Errorf("Expected no error, but got %q", err)
	}

	if rec.Msg.Answer[0].(*dns.HINFO).Cpu != "ANY obsoleted" {
		t.Errorf("Expected HINFO, but got %q", rec.Msg.Answer[0].(*dns.HINFO).Cpu)
	}
}

func TestAnyNonANYQuery(t *testing.T) {
	tests := []struct {
		name  string
		qtype uint16
	}{
		{"A query", dns.TypeA},
		{"AAAA query", dns.TypeAAAA},
		{"MX query", dns.TypeMX},
		{"TXT query", dns.TypeTXT},
		{"CNAME query", dns.TypeCNAME},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := new(dns.Msg)
			req.SetQuestion("example.org.", tt.qtype)

			nextCalled := false
			a := &Any{
				Next: test.HandlerFunc(func(_ctx context.Context, _w dns.ResponseWriter, _r *dns.Msg) (int, error) {
					nextCalled = true
					return 0, nil
				}),
			}

			rec := dnstest.NewRecorder(&test.ResponseWriter{})
			_, err := a.ServeDNS(context.TODO(), rec, req)

			if err != nil {
				t.Errorf("Expected no error, but got %q", err)
			}

			if !nextCalled {
				t.Error("Expected Next handler to be called for non-ANY query")
			}
		})
	}
}
