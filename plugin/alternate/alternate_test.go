package alternate

import (
	"testing"

	"context"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// testHandler implements HandlerWithCallbacks to mock handler
type testHandler struct {
	rcode       int
	called      int
	lastIsEdns0 bool
}

// newTestHandler sets up handler (forward plugin) mock. It returns rcode defined in parameter.
func newTestHandler(rcode int) *testHandler {
	return &testHandler{rcode: rcode}
}

func (h *testHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	h.lastIsEdns0 = r.IsEdns0() != nil
	h.called++
	ret := new(dns.Msg)
	ret.SetReply(r)
	ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
	ret.Rcode = h.rcode
	w.WriteMsg(ret)
	return 0, nil
}
func (h *testHandler) Name() string      { return "testHandler" }
func (h *testHandler) OnStartup() error  { return nil }
func (h *testHandler) OnShutdown() error { return nil }

// stubNextHandler is used to simulate a rewrite and forward plugin.
// It returns a stub Handler that returns the rcode and err specified when invoked.
// Also it adds edns0 option to given request.
func stubNextHandler(rcode int, err error) test.Handler {
	return test.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		returnCode := rcode
		if r == nil {
			r = &dns.Msg{}
		}
		r.SetEdns0(4096, false)
		if rcode != dns.RcodeServerFailure {
			r.MsgHdr.Rcode = rcode
			returnCode = dns.RcodeSuccess
			w.WriteMsg(r)
		} else {
			w.WriteMsg(nil)
		}
		return returnCode, err
	})
}

// makeTestCall makes test call to handler
func makeTestCall(handler *Alternate) (*dnstest.Recorder, int, error) {
	// Prepare query and make a call
	ctx := context.TODO()
	req := &dns.Msg{
		Question: []dns.Question{{
			Name:   "abc.com.",
			Qclass: dns.ClassINET,
			Qtype:  dns.TypeA,
		}},
	}

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	rcode, err := handler.ServeDNS(ctx, rec, req)
	return rec, rcode, err
}

// Test case for alternate
type alternateTestCase struct {
	nextRcode     int // rcode to be returned by the stub Handler
	expectedRcode int // this is expected rcode by test handler (forward plugin)
	called        int // this is expected number of calls reached test alternate server
}

func TestAlternate(t *testing.T) {
	testCases := []alternateTestCase{
		{
			nextRcode:     dns.RcodeNXRrset,
			expectedRcode: dns.RcodeRefused,
			called:        1,
		},
		{
			nextRcode:     dns.RcodeServerFailure,
			expectedRcode: dns.RcodeRefused,
			called:        1,
		},
		{
			//No such code in table.
			nextRcode:     dns.RcodeBadName,
			expectedRcode: dns.RcodeBadName, //Remains from nextRcode
			called:        0,
		},
		{
			//No such code in table.
			nextRcode:     dns.RcodeRefused,
			expectedRcode: dns.RcodeRefused, //Remains from nextRcode
			called:        0,
		},
	}

	for testNum, tc := range testCases {
		// mocked Forward for servicing a specific rcode
		h := newTestHandler(dns.RcodeRefused)

		handler := New()
		// create stub handler to return the test rcode
		handler.Next = stubNextHandler(tc.nextRcode, nil)
		// add rules
		handler.rules = map[int]rule{
			dns.RcodeNXRrset:       {handler: h},
			dns.RcodeServerFailure: {handler: h},
		}

		// Prepare query and make a call
		rec, rcode, err := makeTestCall(handler)

		// Ensure that no errors returned
		if rcode != dns.RcodeSuccess || err != nil {
			t.Errorf("Test '%d': Alternate returned code '%d' error '%v'. Expected RcodeSuccess (0) and no error",
				testNum, rcode, err)
		}

		// Ensure that overall returned code is correct
		if rec.Rcode != tc.expectedRcode {
			t.Errorf("Test '%d': Alternate returned code '%v (%d)', but expected '%v (%d)'",
				testNum, dns.RcodeToString[rec.Rcode], rec.Rcode, dns.RcodeToString[tc.expectedRcode], tc.expectedRcode)
		}

		// Ensure that server was called required number of times
		if h.called != tc.called {
			t.Errorf("Test '%d': Server expected to be called %d time(s) but called %d times(s)",
				testNum, tc.called, h.called)
		}
	}
}

func TestAlternateMultipleCalls(t *testing.T) {
	testCases := []struct {
		nextRcode int
		called    int
	}{
		{nextRcode: dns.RcodeNXRrset, called: 10},
		// No RcodeBadName in table. So, no calls to test server made.
		{nextRcode: dns.RcodeBadName, called: 0},
	}

	for testNum, tc := range testCases {
		// mocked Forward for servicing a specific rcode
		h := newTestHandler(dns.RcodeRefused)

		handler := New()
		// create stub handler to return the test rcode
		handler.Next = stubNextHandler(tc.nextRcode, nil)
		// add rules
		handler.rules = map[int]rule{
			dns.RcodeNXRrset:       {handler: h},
			dns.RcodeServerFailure: {handler: h},
		}

		// Prepare query and make 10 calls
		for i := 0; i < 10; i++ {
			makeTestCall(handler)
		}

		// Ensure that server was called required number of times
		if h.called != tc.called {
			t.Errorf("Test '%d': Server expected to be called %d time(s) but called %d times(s)",
				testNum, tc.called, h.called)
		}
	}
}

func TestAlternateOriginal(t *testing.T) {
	testCases := []struct {
		nextRcode int
		isEdns0   bool
	}{
		// isEdns0 is rewrited by original
		{nextRcode: dns.RcodeNXRrset, isEdns0: false},
		// RcodeServerFailure hasn't original flag set. isEdns0 remains the same
		{nextRcode: dns.RcodeServerFailure, isEdns0: true},
	}

	for testNum, tc := range testCases {
		// mocked Forward for servicing a specific rcode
		h := newTestHandler(dns.RcodeRefused)

		handler := New()
		// One of rules has "original" flag set
		handler.original = true
		// create stub handler to return the test rcode
		handler.Next = stubNextHandler(tc.nextRcode, nil)
		// add rules
		handler.rules = map[int]rule{
			dns.RcodeNXRrset:       {original: true, handler: h},
			dns.RcodeServerFailure: {handler: h},
		}

		// Prepare query and make a call
		makeTestCall(handler)

		// Ensure edns0 option has expected state
		if h.lastIsEdns0 != tc.isEdns0 {
			if tc.isEdns0 {
				t.Errorf("Test '%d': Server expected to recieve Edns0, but didn't", testNum)
			} else {
				t.Errorf("Test '%d': Server not expected to recieve Edns0, but received it", testNum)
			}
		}
	}
}
