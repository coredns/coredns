package nsid

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/cache"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/plugin/whoami"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestNsid(t *testing.T) {
	em := Nsid{
		Data: "NSID",
	}

	tests := []struct {
		next          plugin.Handler
		qname         string
		qtype         uint16
		expectedCode  int
		expectedReply string
		expectedErr   error
	}{
		{
			next:          whoami.Whoami{},
			qname:         ".",
			expectedCode:  dns.RcodeSuccess,
			expectedReply: hex.EncodeToString([]byte("NSID")),
			expectedErr:   nil,
		},
	}

	ctx := context.TODO()

	for i, tc := range tests {
		req := new(dns.Msg)
		if tc.qtype == 0 {
			tc.qtype = dns.TypeA
		}
		req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)
		req.Question[0].Qclass = dns.ClassINET

		req.SetEdns0(4096, false)
		option := req.Extra[0].(*dns.OPT)
		option.Option = append(option.Option, &dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: ""})
		em.Next = tc.next

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		code, err := em.ServeDNS(ctx, rec, req)

		if err != tc.expectedErr {
			t.Errorf("Test %d: Expected error %v, but got %v", i, tc.expectedErr, err)
		}
		if code != tc.expectedCode {
			t.Errorf("Test %d: Expected status code %d, but got %d", i, tc.expectedCode, code)
		}
		if tc.expectedReply != "" {
			for _, extra := range rec.Msg.Extra {
				if option, ok := extra.(*dns.OPT); ok {
					e := option.Option[0].(*dns.EDNS0_NSID)
					if e.Nsid != tc.expectedReply {
						t.Errorf("Test %d: Expected answer %s, but got %s", i, tc.expectedReply, e.Nsid)
					}
				}
			}
		}
	}
}

func TestNsidCache(t *testing.T) {
	em := Nsid{
		Data: "NSID",
	}
	c := cache.New()

	tests := []struct {
		next          plugin.Handler
		qname         string
		qtype         uint16
		expectedCode  int
		expectedReply string
		expectedErr   error
	}{
		{
			next:          whoami.Whoami{},
			qname:         ".",
			expectedCode:  dns.RcodeSuccess,
			expectedReply: hex.EncodeToString([]byte("NSID")),
			expectedErr:   nil,
		},
	}

	ctx := context.TODO()

	for i, tc := range tests {
		req := new(dns.Msg)
		if tc.qtype == 0 {
			tc.qtype = dns.TypeA
		}
		req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)
		req.Question[0].Qclass = dns.ClassINET

		req.SetEdns0(4096, false)
		option := req.Extra[0].(*dns.OPT)
		option.Option = append(option.Option, &dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: ""})
		em.Next = tc.next

		rec := dnstest.NewRecorder(&test.ResponseWriter{})

		c.Next = em
		code, err := c.ServeDNS(ctx, rec, req)

		if err != tc.expectedErr {
			t.Errorf("Test %d: Expected error %v, but got %v", i, tc.expectedErr, err)
		}
		if code != tc.expectedCode {
			t.Errorf("Test %d: Expected status code %d, but got %d", i, tc.expectedCode, code)
		}
		if tc.expectedReply != "" {
			for _, extra := range rec.Msg.Extra {
				if option, ok := extra.(*dns.OPT); ok {
					e := option.Option[0].(*dns.EDNS0_NSID)
					if e.Nsid != tc.expectedReply {
						t.Errorf("Test %d: Expected answer %s, but got %s", i, tc.expectedReply, e.Nsid)
					}
				}
			}
		}
	}
}

func TestNsidCopiesDo(t *testing.T) {
	for _, do := range []bool{false, true} {
		req := new(dns.Msg)
		req.SetQuestion(".", dns.TypeA)
		req.SetEdns0(4096, do)
		option := req.Extra[0].(*dns.OPT)
		option.Option = append(option.Option, &dns.EDNS0_NSID{Code: dns.EDNS0NSID})

		em := Nsid{Data: "NSID", Next: whoami.Whoami{}}
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		if _, err := em.ServeDNS(context.TODO(), request.NewScrubWriter(req, rec), req); err != nil {
			t.Fatalf("DO=%t: unexpected error: %v", do, err)
		}

		opt := rec.Msg.IsEdns0()
		if opt == nil {
			t.Fatalf("DO=%t: expected an OPT record in the response", do)
		}
		if opt.Do() != do {
			t.Errorf("DO=%t: expected DO bit %t in the response, got %t", do, do, opt.Do())
		}
	}
}
