package tsig

import (
	"context"
	"fmt"
	"slices"
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
		reqOpCodes  opCodes
		qType       uint16
		opcode      int
		extra       []dns.RR
		qTsig       bool
		allTypes    bool
		allOpcodes  bool
		expectRcode int
		expectTsig  bool
		statusError bool
	}{
		{
			zones:       []string{"."},
			allTypes:    true,
			qType:       dns.TypeA,
			opcode:      dns.OpcodeQuery,
			qTsig:       true,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  true,
		},
		{
			zones:       []string{"."},
			allTypes:    true,
			qType:       dns.TypeA,
			opcode:      dns.OpcodeQuery,
			qTsig:       false,
			expectRcode: dns.RcodeRefused,
			expectTsig:  false,
		},
		{
			zones:       []string{"another.domain."},
			allTypes:    true,
			qType:       dns.TypeA,
			opcode:      dns.OpcodeQuery,
			qTsig:       false,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  false,
		},
		{
			zones:       []string{"another.domain."},
			allTypes:    true,
			qType:       dns.TypeA,
			opcode:      dns.OpcodeQuery,
			qTsig:       true,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  false,
		},
		{
			zones:       []string{"."},
			reqTypes:    qTypes{dns.TypeAXFR: {}},
			qType:       dns.TypeAXFR,
			opcode:      dns.OpcodeQuery,
			qTsig:       true,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  true,
		},
		{
			zones:       []string{"."},
			reqTypes:    qTypes{},
			qType:       dns.TypeA,
			opcode:      dns.OpcodeQuery,
			qTsig:       false,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  false,
		},
		{
			zones:       []string{"."},
			reqTypes:    qTypes{},
			qType:       dns.TypeA,
			opcode:      dns.OpcodeQuery,
			qTsig:       true,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  true,
		},
		{
			zones:       []string{"."},
			allTypes:    true,
			qType:       dns.TypeA,
			opcode:      dns.OpcodeQuery,
			qTsig:       true,
			expectRcode: dns.RcodeNotAuth,
			expectTsig:  true,
			statusError: true,
		},
		// Opcode-based tests
		{
			zones:       []string{"."},
			reqOpCodes:  opCodes{dns.OpcodeUpdate: {}},
			qType:       dns.TypeSOA,
			opcode:      dns.OpcodeUpdate,
			qTsig:       true,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  true,
		},
		{
			zones:       []string{"."},
			reqOpCodes:  opCodes{dns.OpcodeUpdate: {}},
			qType:       dns.TypeSOA,
			opcode:      dns.OpcodeUpdate,
			qTsig:       false,
			expectRcode: dns.RcodeRefused,
			expectTsig:  false,
		},
		{
			zones:       []string{"."},
			reqOpCodes:  opCodes{dns.OpcodeUpdate: {}},
			qType:       dns.TypeA,
			opcode:      dns.OpcodeQuery,
			qTsig:       false,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  false,
		},
		{
			zones:       []string{"."},
			reqOpCodes:  opCodes{dns.OpcodeNotify: {}},
			qType:       dns.TypeSOA,
			opcode:      dns.OpcodeNotify,
			qTsig:       true,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  true,
		},
		// Combined qtype and opcode requirement
		{
			zones:       []string{"."},
			reqTypes:    qTypes{dns.TypeAXFR: {}},
			reqOpCodes:  opCodes{dns.OpcodeUpdate: {}},
			qType:       dns.TypeA,
			opcode:      dns.OpcodeUpdate,
			qTsig:       true,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  true,
		},
		// allOpcodes test
		{
			zones:       []string{"."},
			allOpcodes:  true,
			qType:       dns.TypeA,
			opcode:      dns.OpcodeQuery,
			qTsig:       true,
			expectRcode: dns.RcodeSuccess,
			expectTsig:  true,
		},
		{
			zones:       []string{"."},
			allOpcodes:  true,
			qType:       dns.TypeA,
			opcode:      dns.OpcodeQuery,
			qTsig:       false,
			expectRcode: dns.RcodeRefused,
			expectTsig:  false,
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			tsig := TSIGServer{
				Zones:      tc.zones,
				allTypes:   tc.allTypes,
				allOpcodes: tc.allOpcodes,
				types:      tc.reqTypes,
				opcodes:    tc.reqOpCodes,
				Next:       testHandler(),
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
			r.Opcode = tc.opcode
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
		reqError          int
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
		{
			desc:              "Client Set Error",
			tsigErr:           nil,
			reqError:          dns.RcodeBadKey,
			expectRcode:       dns.RcodeSuccess,
			expectError:       dns.RcodeSuccess,
			expectOtherLength: 0,
			expectTimeSigned:  0,
		},
	}

	tsig := TSIGServer{
		Zones:    []string{"."},
		allTypes: true,
		Next:     testHandler(),
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := context.TODO()

			var w = dnstest.NewRecorder(&ErrWriter{err: tc.tsigErr})

			r := new(dns.Msg)
			r.SetQuestion("test.example.", dns.TypeA)
			r.SetTsig("test.key.", dns.HmacSHA256, 300, clientNow)

			rtsig := r.IsTsig()
			rtsig.Error = uint16(tc.reqError)
			// set a fake MAC and Size in request
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

func TestServeDNSTsigNext(t *testing.T) {
	cases := []struct {
		desc         string
		zones        []string
		tsigRequired bool
		tsigStatus   error
		reqExtra     []dns.RR
		reqSigned    bool
		expectExtra  []uint16
		expectNext   int
	}{
		{
			desc:         "Optional TSIG",
			zones:        []string{"."},
			tsigRequired: false,
			reqExtra:     []dns.RR{test.OPT(42, true)},
			reqSigned:    false,
			expectExtra:  []uint16{dns.TypeOPT},
			expectNext:   1,
		},
		{
			desc:         "Missing TSIG",
			zones:        []string{"."},
			tsigRequired: true,
			tsigStatus:   nil,
			reqExtra:     []dns.RR{test.OPT(42, true)},
			reqSigned:    false,
			expectNext:   0,
		},
		{
			desc:        "Bad Zone",
			zones:       []string{"another.domain."},
			reqExtra:    []dns.RR{test.OPT(42, true)},
			reqSigned:   true,
			expectExtra: []uint16{dns.TypeOPT, dns.TypeTSIG},
			expectNext:  1,
		},
		{
			desc:         "Bad Status",
			zones:        []string{"."},
			tsigRequired: true,
			tsigStatus:   dns.ErrSig,
			reqExtra:     []dns.RR{test.OPT(42, true)},
			reqSigned:    true,
			expectNext:   0,
		},
		{
			desc:         "Success",
			zones:        []string{"."},
			tsigRequired: true,
			reqExtra:     []dns.RR{test.OPT(42, true)},
			reqSigned:    true,
			expectExtra:  []uint16{dns.TypeOPT},
			expectNext:   1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			h := testHandler()
			var nextCalled int
			tsig := TSIGServer{
				Zones:    tc.zones,
				allTypes: tc.tsigRequired,
				Next: test.HandlerFunc(func(_ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
					nextCalled++
					if !slices.EqualFunc(r.Extra, tc.expectExtra, func(rr dns.RR, t uint16) bool { return rr.Header().Rrtype == t }) {
						t.Errorf("expected %v, got %v", tc.expectExtra, r.Extra)
					}
					return h(_ctx, w, r)
				}),
			}

			ctx := context.TODO()

			w := dnstest.NewRecorder(&ErrWriter{err: tc.tsigStatus})

			r := new(dns.Msg)
			r.SetQuestion("test.example.", dns.TypeA)
			r.Extra = tc.reqExtra
			if tc.reqSigned {
				r.SetTsig("test.key.", dns.HmacSHA256, 300, time.Now().Unix())
			}

			_, err := tsig.ServeDNS(ctx, w, r)
			if err != nil {
				t.Fatal(err)
			}
			if nextCalled != tc.expectNext {
				t.Errorf("expected next plugin called")
			}
		})
	}
}

func testHandler() test.HandlerFunc {
	return func(_ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
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
