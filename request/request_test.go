package request

import (
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestRequestDo(t *testing.T) {
	st := testRequest()

	st.Do()
	if !st.do {
		t.Errorf("Expected st.do to be set")
	}
}

func TestRequestRemote(t *testing.T) {
	st := testRequest()
	if st.IP() != "10.240.0.1" {
		t.Errorf("Wrong IP from request")
	}
	p := st.Port()
	if p == "" {
		t.Errorf("Failed to get Port from request")
	}
	if p != "40212" {
		t.Errorf("Wrong port from request")
	}
}

// TestRequestLocal tests LocalIP and LocalPort methods
func TestRequestLocal(t *testing.T) {
	st := testRequest()
	if st.LocalIP() != "127.0.0.1" {
		t.Errorf("Wrong LocalIP from request, got %s", st.LocalIP())
	}
	p := st.LocalPort()
	if p == "" {
		t.Errorf("Failed to get LocalPort from request")
	}
	if p != "53" {
		t.Errorf("Wrong LocalPort from request, got %s", p)
	}
}

// TestRequestAddrs tests RemoteAddr and LocalAddr methods
func TestRequestAddrs(t *testing.T) {
	st := testRequest()
	remote := st.RemoteAddr()
	if remote != "10.240.0.1:40212" {
		t.Errorf("Wrong RemoteAddr from request, got %s", remote)
	}
	local := st.LocalAddr()
	if local != "127.0.0.1:53" {
		t.Errorf("Wrong LocalAddr from request, got %s", local)
	}
}

// TestRequestProto tests Proto and Family methods together
func TestRequestProto(t *testing.T) {
	st := testRequest()
	proto := st.Proto()
	if proto != "udp" {
		t.Errorf("Expected proto to be udp, got %s", proto)
	}
	family := st.Family()
	if family != 1 {
		t.Errorf("Expected family to be 1 (IPv4), got %d", family)
	}
}

// TestRequestSizeAndDo tests the SizeAndDo method
func TestRequestSizeAndDo(t *testing.T) {
	st := testRequest()
	m := new(dns.Msg)

	// Test with no OPT in the response
	modified := st.SizeAndDo(m)
	if !modified {
		t.Errorf("Expected SizeAndDo to return true")
	}
	if m.IsEdns0() == nil {
		t.Errorf("Expected OPT record to be added to response")
	}

	// Test with existing OPT in the response
	m = new(dns.Msg)
	opt := new(dns.OPT)
	opt.Hdr.Name = "."
	opt.Hdr.Rrtype = dns.TypeOPT
	opt.SetUDPSize(2048)
	m.Extra = append(m.Extra, opt)

	modified = st.SizeAndDo(m)
	if !modified {
		t.Errorf("Expected SizeAndDo to return true")
	}
	if m.IsEdns0() == nil {
		t.Errorf("Expected OPT record to remain in response")
	}
	if m.IsEdns0().UDPSize() != 4096 {
		t.Errorf("Expected UDP size to be updated to 4096, got %d", m.IsEdns0().UDPSize())
	}
}

// TestRequestNewWithQuestion tests the NewWithQuestion method
func TestRequestNewWithQuestion(t *testing.T) {
	st := testRequest()
	newReq := st.NewWithQuestion("example.org.", dns.TypeMX)

	if newReq.Name() != "example.org." {
		t.Errorf("Expected new request name to be example.org., got %s", newReq.Name())
	}
	if newReq.QType() != dns.TypeMX {
		t.Errorf("Expected new request type to be MX, got %d", newReq.QType())
	}

	// Original request should be unchanged
	if st.Name() != "example.com." {
		t.Errorf("Expected original request to be unchanged, got %s", st.Name())
	}
	if st.QType() != dns.TypeA {
		t.Errorf("Expected original request type to remain A, got %d", st.QType())
	}
}

func TestRequestMalformed(t *testing.T) {
	m := new(dns.Msg)
	st := Request{Req: m}

	if x := st.QType(); x != 0 {
		t.Errorf("Expected 0 Qtype, got %d", x)
	}

	if x := st.QClass(); x != 0 {
		t.Errorf("Expected 0 QClass, got %d", x)
	}

	if x := st.QName(); x != "." {
		t.Errorf("Expected . Qname, got %s", x)
	}

	if x := st.Name(); x != "." {
		t.Errorf("Expected . Name, got %s", x)
	}

	if x := st.Type(); x != "" {
		t.Errorf("Expected empty Type, got %s", x)
	}

	if x := st.Class(); x != "" {
		t.Errorf("Expected empty Class, got %s", x)
	}
}

func TestRequestScrubAnswer(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("large.example.com.", dns.TypeSRV)
	req := Request{W: &test.ResponseWriter{}, Req: m}

	reply := new(dns.Msg)
	reply.SetReply(m)
	for i := 1; i < 200; i++ {
		reply.Answer = append(reply.Answer, test.SRV(
			fmt.Sprintf("large.example.com. 10 IN SRV 0 0 80 10-0-0-%d.default.pod.k8s.example.com.", i)))
	}

	req.Scrub(reply)
	if want, got := req.Size(), reply.Len(); want < got {
		t.Errorf("Want scrub to reduce message length below %d bytes, got %d bytes", want, got)
	}
	if !reply.Truncated {
		t.Errorf("Want scrub to set truncated bit")
	}
}

func TestRequestScrubExtra(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("large.example.com.", dns.TypeSRV)
	req := Request{W: &test.ResponseWriter{}, Req: m}

	reply := new(dns.Msg)
	reply.SetReply(m)
	for i := 1; i < 200; i++ {
		reply.Extra = append(reply.Extra, test.SRV(
			fmt.Sprintf("large.example.com. 10 IN SRV 0 0 80 10-0-0-%d.default.pod.k8s.example.com.", i)))
	}

	req.Scrub(reply)
	if want, got := req.Size(), reply.Len(); want < got {
		t.Errorf("Want scrub to reduce message length below %d bytes, got %d bytes", want, got)
	}
	if !reply.Truncated {
		t.Errorf("Want scrub to set truncated bit")
	}
}

func TestRequestScrubExtraEdns0(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("large.example.com.", dns.TypeSRV)
	m.SetEdns0(4096, true)
	req := Request{W: &test.ResponseWriter{}, Req: m}

	reply := new(dns.Msg)
	reply.SetReply(m)
	for i := 1; i < 200; i++ {
		reply.Extra = append(reply.Extra, test.SRV(
			fmt.Sprintf("large.example.com. 10 IN SRV 0 0 80 10-0-0-%d.default.pod.k8s.example.com.", i)))
	}

	req.Scrub(reply)
	if want, got := req.Size(), reply.Len(); want < got {
		t.Errorf("Want scrub to reduce message length below %d bytes, got %d bytes", want, got)
	}
	if !reply.Truncated {
		t.Errorf("Want scrub to set truncated bit")
	}
}

func TestRequestScrubExtraRegression(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("large.example.com.", dns.TypeSRV)
	m.SetEdns0(2048, true)
	req := Request{W: &test.ResponseWriter{}, Req: m}

	reply := new(dns.Msg)
	reply.SetReply(m)
	for i := 1; i < 33; i++ {
		reply.Answer = append(reply.Answer, test.SRV(
			fmt.Sprintf("large.example.com. 10 IN SRV 0 0 80 10-0-0-%d.default.pod.k8s.example.com.", i)))
	}
	for i := 1; i < 33; i++ {
		reply.Extra = append(reply.Extra, test.A(
			fmt.Sprintf("10-0-0-%d.default.pod.k8s.example.com. 10 IN A 10.0.0.%d", i, i)))
	}

	reply = req.Scrub(reply)
	if want, got := req.Size(), reply.Len(); want < got {
		t.Errorf("Want scrub to reduce message length below %d bytes, got %d bytes", want, got)
	}
	if !reply.Truncated {
		t.Errorf("Want scrub to set truncated bit")
	}
}

func TestTruncation(t *testing.T) {
	for bufsize := 1024; bufsize <= 4096; bufsize += 12 {
		m := new(dns.Msg)
		m.SetQuestion("http.service.tcp.srv.k8s.example.org", dns.TypeSRV)
		m.SetEdns0(uint16(bufsize), true)
		req := Request{W: &test.ResponseWriter{}, Req: m}

		reply := new(dns.Msg)
		reply.SetReply(m)

		for i := range 61 {
			reply.Answer = append(reply.Answer, test.SRV(fmt.Sprintf("http.service.tcp.srv.k8s.example.org. 5 IN SRV 0 0 80 10-144-230-%d.default.pod.k8s.example.org.", i)))
		}

		for i := range 5 {
			reply.Extra = append(reply.Extra, test.A(fmt.Sprintf("ip-10-10-52-5%d.subdomain.example.org. 5 IN A 10.10.52.5%d", i, i)))
		}

		for i := range 5 {
			reply.Ns = append(reply.Ns, test.NS(fmt.Sprintf("srv.subdomain.example.org. 5 IN NS ip-10-10-33-6%d.subdomain.example.org.", i)))
		}

		req.Scrub(reply)
		want, got := req.Size(), reply.Len()
		if want < got {
			t.Fatalf("Want scrub to reduce message length below %d bytes, got %d bytes", want, got)
		}
	}
}

func TestRequestScrubAnswerExact(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("large.example.com.", dns.TypeSRV)
	m.SetEdns0(867, false) // Bit fiddly, but this hits the rl == size break clause in Scrub, 52 RRs should remain.
	req := Request{W: &test.ResponseWriter{}, Req: m}

	reply := new(dns.Msg)
	reply.SetReply(m)
	for i := 1; i < 200; i++ {
		reply.Answer = append(reply.Answer, test.A(fmt.Sprintf("large.example.com. 10 IN A 127.0.0.%d", i)))
	}

	req.Scrub(reply)
	if want, got := req.Size(), reply.Len(); want < got {
		t.Errorf("Want scrub to reduce message length below %d bytes, got %d bytes", want, got)
	}
}

func TestRequestMatch(t *testing.T) {
	st := testRequest()
	reply := new(dns.Msg)
	reply.Response = true

	reply.SetQuestion("example.com.", dns.TypeMX)
	if b := st.Match(reply); b {
		t.Errorf("Failed to match %s %d, got %t, expected %t", "example.com.", dns.TypeMX, b, false)
	}

	reply.SetQuestion("example.com.", dns.TypeA)
	if b := st.Match(reply); !b {
		t.Errorf("Failed to match %s %d, got %t, expected %t", "example.com.", dns.TypeA, b, true)
	}

	reply.SetQuestion("example.org.", dns.TypeA)
	if b := st.Match(reply); b {
		t.Errorf("Failed to match %s %d, got %t, expected %t", "example.org.", dns.TypeA, b, false)
	}
}

func BenchmarkRequestDo(b *testing.B) {
	st := testRequest()

	for range b.N {
		st.Do()
	}
}

func BenchmarkRequestSize(b *testing.B) {
	st := testRequest()

	for range b.N {
		st.Size()
	}
}

func BenchmarkRequestScrub(b *testing.B) {
	st := testRequest()

	reply := new(dns.Msg)
	reply.SetReply(st.Req)
	for i := 1; i < 33; i++ {
		reply.Answer = append(reply.Answer, test.SRV(
			fmt.Sprintf("large.example.com. 10 IN SRV 0 0 80 10-0-0-%d.default.pod.k8s.example.com.", i)))
	}
	for i := 1; i < 33; i++ {
		reply.Extra = append(reply.Extra, test.A(
			fmt.Sprintf("10-0-0-%d.default.pod.k8s.example.com. 10 IN A 10.0.0.%d", i, i)))
	}

	b.ResetTimer()
	for range b.N {
		st.Scrub(reply.Copy())
	}
}

func testRequest() Request {
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	m.SetEdns0(4096, true)
	return Request{W: &test.ResponseWriter{}, Req: m}
}

func TestRequestClear(t *testing.T) {
	st := testRequest()
	if st.IP() != "10.240.0.1" {
		t.Errorf("Wrong IP from request")
	}
	p := st.Port()
	if p == "" {
		t.Errorf("Failed to get Port from request")
	}
	st.Clear()
	if st.ip != "" {
		t.Errorf("Expected st.ip to be cleared after Clear")
	}

	if st.port != "" {
		t.Errorf("Expected st.port to be cleared after Clear")
	}
}
