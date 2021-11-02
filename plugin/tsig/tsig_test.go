package tsig

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
)

func TestServeDNS(t *testing.T) {
	cases := []struct{
		zones []string
		reqTypes qTypes
		qType uint16
		qTsig, all bool
		expectRcode int
		expectTsig bool
		statusError bool
	}{
		{
			zones: []string{"."},
			all: true,
			qType: dns.TypeA,
			qTsig: true,
			expectRcode: dns.RcodeSuccess,
			expectTsig: true,
		},
		{
			zones: []string{"."},
			all: true,
			qType: dns.TypeA,
			qTsig: false,
			expectRcode: dns.RcodeRefused,
			expectTsig: false,
		},
		{
			zones: []string{"another.domain."},
			all: true,
			qType: dns.TypeA,
			qTsig: false,
			expectRcode: dns.RcodeSuccess,
			expectTsig: false,
		},
		{
			zones: []string{"another.domain."},
			all: true,
			qType: dns.TypeA,
			qTsig: true,
			expectRcode: dns.RcodeSuccess,
			expectTsig: false,
		},
		{
			zones: []string{"."},
			reqTypes: qTypes{dns.TypeAXFR: {}},
			qType: dns.TypeAXFR,
			qTsig: true,
			expectRcode: dns.RcodeSuccess,
			expectTsig: true,
		},
		{
			zones: []string{"."},
			reqTypes: qTypes{},
			qType: dns.TypeA,
			qTsig: false,
			expectRcode: dns.RcodeSuccess,
			expectTsig: false,
		},
		{
			zones: []string{"."},
			reqTypes: qTypes{},
			qType: dns.TypeA,
			qTsig: true,
			expectRcode: dns.RcodeSuccess,
			expectTsig: true,
		},
		{
			zones: []string{"."},
			all: true,
			qType: dns.TypeA,
			qTsig: true,
			expectRcode: dns.RcodeNotAuth,
			expectTsig: true,
			statusError: true,
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			tsig := TSIGServer{
				Zones: tc.zones,
				all:   tc.all,
				types: tc.reqTypes,
				Next:  testHandler(),
			}

			ctx := context.TODO()

			var w *dnstest.Recorder
			if tc.statusError {
				w = dnstest.NewRecorder(&SigErrWriter{})
			} else {
				w = dnstest.NewRecorder(&test.ResponseWriter{})
			}
			r := new(dns.Msg)
			r.SetQuestion("test.example.", tc.qType)
			if tc.qTsig {
				r.SetTsig("test.key.", dns.HmacSHA256, 300, time.Now().Unix())
			}

			_, err := tsig.ServeDNS(ctx, w, r)
			if err != nil {
				t.Fatal(err)
			}

			if w.Msg.Rcode != tc.expectRcode {
				t.Fatalf("expected rcode %v, got %v", tc.expectTsig, w.Msg.Rcode)
			}

			if ts := w.Msg.IsTsig(); ts == nil && tc.expectTsig {
				t.Fatal("expected TSIG in response")
			}
			if ts := w.Msg.IsTsig(); ts != nil && !tc.expectTsig {
				t.Fatal("expected no TSIG in response")
			}
		})
	}
}

func testHandler() test.HandlerFunc {
	return func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		state := request.Request{W: w, Req: r}
		qname := state.Name()
		m := new(dns.Msg)
		rcode := dns.RcodeServerFailure
		if qname == "test.example." {
			m.SetReply(r)
			rr := test.A("test.example.  300  IN  A  1.2.3.48")
			m.Answer = []dns.RR{rr}
			m.Authoritative = true
			rcode = dns.RcodeSuccess
		}
		m.SetRcode(r, rcode)
		w.WriteMsg(m)
		return rcode, nil
	}
}

// a test.ResponseWriter that always returns a TSIG status error
type SigErrWriter struct {
	test.ResponseWriter
}

// TsigStatus always returns an error.
func (t *SigErrWriter) TsigStatus() error { return dns.ErrSig }