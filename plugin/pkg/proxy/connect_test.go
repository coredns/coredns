package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/doh"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

const (
	testMsgExpectedError      = "expected error"
	testMsgUnexpectedNilError = "unexpected nil error"
	testMsgWrongError         = "wrong error message"
)

// TestDial_TransportStopped_InitialCheck tests that Dial returns ErrTransportStopped
// if the transport is stopped before Dial is called.
func TestDial_TransportStopped_InitialCheck(t *testing.T) {
	tr := newTransport("test_initial_stop", "127.0.0.1:0")
	tr.Start()

	tr.Stop()
	time.Sleep(50 * time.Millisecond) // Ensure connManager processes stop and exits

	_, _, err := tr.Dial("udp")
	if err == nil {
		t.Fatalf("%s: %s", testMsgExpectedError, testMsgUnexpectedNilError)
	}
	if err.Error() != ErrTransportStopped {
		t.Errorf("%s: got '%v', want '%s'", testMsgWrongError, err, ErrTransportStopped)
	}
}

// TestDial_MultipleCallsAfterStop tests that multiple Dial calls after Stop
// consistently return ErrTransportStopped.
func TestDial_MultipleCallsAfterStop(t *testing.T) {
	tr := newTransport("test_multiple_after_stop", "127.0.0.1:0")
	tr.Start()

	tr.Stop()
	time.Sleep(50 * time.Millisecond)

	for i := range 3 {
		_, _, err := tr.Dial("udp")
		if err == nil {
			t.Errorf("Attempt %d: %s: %s", i+1, testMsgExpectedError, testMsgUnexpectedNilError)
			continue
		}
		if err.Error() != ErrTransportStopped {
			t.Errorf("Attempt %d: %s: got '%v', want '%s'", i+1, testMsgWrongError, err, ErrTransportStopped)
		}
	}
}

// TestConnectHTTPSReportsTCP verifies that Connect reports the transport it
// actually used to reach the upstream. For an HTTPS upstream that is always
// "tcp", even when the downstream client connected over UDP.
func TestConnectHTTPSReportsTCP(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		msg, err := doh.RequestToMsg(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		ret := new(dns.Msg)
		reply := ret.SetReply(msg)
		reply.Answer = append(reply.Answer, test.A("example.org. IN A 127.0.0.1"))

		buf, err := reply.Pack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", doh.MimeType)
		w.Write(buf)
	}))
	defer s.Close()

	p := NewProxy("TestConnectHTTPSReportsTCP", s.URL, transport.HTTPS)
	p.SetHTTPClient(s.Client())

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)

	// The downstream client connected over UDP.
	req := request.Request{W: &test.ResponseWriter{TCP: false}, Req: m}

	resp, _, proto, err := p.Connect(context.Background(), req, Options{})
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response, got nil")
	}
	if proto != "tcp" {
		t.Errorf("HTTPS upstream with a UDP downstream client: expected reported proto %q, got %q", "tcp", proto)
	}
}
