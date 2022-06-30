package tsig

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestServeDNS(t *testing.T) {
	cases := []struct {
		zones       []string
		reqTypes    qTypes
		qType       uint16
		qTsig, all  bool
		expectRcode int
		expectTsig  bool
		statusError bool
	}{
		{
			zones:       []string{"."},
			all:         true,
			qType:       dns.TypeA,
			qTsig:       true,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  true,
		},
		{
			zones:       []string{"."},
			all:         true,
			qType:       dns.TypeA,
			qTsig:       false,
			expectRcode: dns.RcodeRefused,
			expectTsig:  false,
		},
		{
			zones:       []string{"another.domain."},
			all:         true,
			qType:       dns.TypeA,
			qTsig:       false,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  false,
		},
		{
			zones:       []string{"another.domain."},
			all:         true,
			qType:       dns.TypeA,
			qTsig:       true,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  false,
		},
		{
			zones:       []string{"."},
			reqTypes:    qTypes{dns.TypeAXFR: {}},
			qType:       dns.TypeAXFR,
			qTsig:       true,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  true,
		},
		{
			zones:       []string{"."},
			reqTypes:    qTypes{},
			qType:       dns.TypeA,
			qTsig:       false,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  false,
		},
		{
			zones:       []string{"."},
			reqTypes:    qTypes{},
			qType:       dns.TypeA,
			qTsig:       true,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  true,
		},
		{
			zones:       []string{"."},
			all:         true,
			qType:       dns.TypeA,
			qTsig:       true,
			expectRcode: dns.RcodeNotAuth,
			expectTsig:  true,
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
				w = dnstest.NewRecorder(&ErrWriter{err: dns.ErrSig})
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
				t.Fatalf("expected rcode %v, got %v", tc.expectRcode, w.Msg.Rcode)
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

func TestServeDNSTsigErrors(t *testing.T) {
	clientNow := time.Now().Unix()

	cases := []struct {
		desc              string
		tsigErr           error
		expectRcode       int
		expectError       int
		expectOtherLength int
		expectTimeSigned  int64
	}{
		{
			desc:              "Unknown Key",
			tsigErr:           dns.ErrSecret,
			expectRcode:       dns.RcodeNotAuth,
			expectError:       dns.RcodeBadKey,
			expectOtherLength: 0,
			expectTimeSigned:  0,
		},
		{
			desc:              "Bad Signature",
			tsigErr:           dns.ErrSig,
			expectRcode:       dns.RcodeNotAuth,
			expectError:       dns.RcodeBadSig,
			expectOtherLength: 0,
			expectTimeSigned:  0,
		},
		{
			desc:              "Bad Time",
			tsigErr:           dns.ErrTime,
			expectRcode:       dns.RcodeNotAuth,
			expectError:       dns.RcodeBadTime,
			expectOtherLength: 6,
			expectTimeSigned:  clientNow,
		},
	}

	tsig := TSIGServer{
		Zones: []string{"."},
		all:   true,
		Next:  testHandler(),
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := context.TODO()

			var w *dnstest.Recorder

			w = dnstest.NewRecorder(&ErrWriter{err: tc.tsigErr})

			r := new(dns.Msg)
			r.SetQuestion("test.example.", dns.TypeA)
			r.SetTsig("test.key.", dns.HmacSHA256, 300, clientNow)

			// set a fake MAC and Size in request
			rtsig := r.IsTsig()
			rtsig.MAC = "0123456789012345678901234567890101234567890123456789012345678901"
			rtsig.MACSize = 32

			_, err := tsig.ServeDNS(ctx, w, r)
			if err != nil {
				t.Fatal(err)
			}

			if w.Msg.Rcode != tc.expectRcode {
				t.Fatalf("expected rcode %v, got %v", tc.expectRcode, w.Msg.Rcode)
			}

			ts := w.Msg.IsTsig()

			if ts == nil {
				t.Fatal("expected TSIG in response")
			}

			if int(ts.Error) != tc.expectError {
				t.Errorf("expected TSIG error code %v, got %v", tc.expectError, ts.Error)
			}

			if len(ts.OtherData)/2 != tc.expectOtherLength {
				t.Errorf("expected Other of length %v, got %v", tc.expectOtherLength, len(ts.OtherData))
			}

			if int(ts.OtherLen) != tc.expectOtherLength {
				t.Errorf("expected OtherLen %v, got %v", tc.expectOtherLength, ts.OtherLen)
			}

			if ts.TimeSigned != uint64(tc.expectTimeSigned) {
				t.Errorf("expected TimeSigned to be %v, got %v", tc.expectTimeSigned, ts.TimeSigned)
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

// a test.ResponseWriter that always returns err as the TSIG status error
type ErrWriter struct {
	err error
	test.ResponseWriter
}

// TsigStatus always returns an error.
func (t *ErrWriter) TsigStatus() error { return t.err }
