package plugin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// mockBackend implements ServiceBackend interface for testing
var _ ServiceBackend = &mockBackend{}

type mockBackend struct {
	mockServices  func(ctx context.Context, state request.Request, exact bool, opt Options) ([]msg.Service, error)
	mockReverse   func(ctx context.Context, state request.Request, exact bool, opt Options) ([]msg.Service, error)
	mockLookup    func(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error)
	mockRecords   func(ctx context.Context, state request.Request, exact bool) ([]msg.Service, error)
	mockNameError func(err error) bool
	minTTL        uint32
	serial        uint32
}

func (m *mockBackend) Serial(_state request.Request) uint32 {
	if m.serial == 0 {
		return uint32(time.Now().Unix())
	}
	return m.serial
}

func (m *mockBackend) MinTTL(_state request.Request) uint32 {
	if m.minTTL == 0 {
		return 30
	}
	return m.minTTL
}

func (m *mockBackend) Services(ctx context.Context, state request.Request, exact bool, opt Options) ([]msg.Service, error) {
	return m.mockServices(ctx, state, exact, opt)
}

func (m *mockBackend) Reverse(ctx context.Context, state request.Request, exact bool, opt Options) ([]msg.Service, error) {
	return m.mockReverse(ctx, state, exact, opt)
}

func (m *mockBackend) Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error) {
	return m.mockLookup(ctx, state, name, typ)
}

func (m *mockBackend) IsNameError(err error) bool {
	if m.mockNameError != nil {
		return m.mockNameError(err)
	}
	return false
}

func (m *mockBackend) Records(ctx context.Context, state request.Request, exact bool) ([]msg.Service, error) {
	return m.mockRecords(ctx, state, exact)
}

func TestNSStateReset(t *testing.T) {
	// Create a mock backend that always returns error
	mock := &mockBackend{
		mockServices: func(_ctx context.Context, _state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			return nil, fmt.Errorf("mock error")
		},
	}
	// Create a test request
	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeNS)
	state := request.Request{
		Req: req,
		W:   &test.ResponseWriter{},
	}

	originalName := state.QName()
	ctx := context.TODO()

	// Call NS function which should fail due to mock error
	records, extra, err := NS(ctx, mock, "example.org.", state, Options{})

	// Verify error is returned
	if err == nil {
		t.Error("Expected error from mock backend, got nil")
	}

	// Verify query name is reset even when an error occurs
	if state.QName() != originalName {
		t.Errorf("Query name not properly reset after error. Expected %s, got %s", originalName, state.QName())
	}

	// Verify no records are returned
	if len(records) != 0 || len(extra) != 0 {
		t.Error("Expected no records returned on error")
	}
}

func TestARecords_Dedup(t *testing.T) {
	b := &mockBackend{
		mockServices: func(_ctx context.Context, _state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			return []msg.Service{
				{Host: "1.2.3.4", TTL: 60},
				{Host: "1.2.3.4", TTL: 60},
				{Host: "::1", TTL: 60},
			}, nil
		},
	}
	req := new(dns.Msg)
	req.SetQuestion("a.example.org.", dns.TypeA)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	recs, tc, err := A(context.Background(), b, "example.org.", state, nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc {
		t.Fatal("unexpected truncation")
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 A record, got %d", len(recs))
	}
	if _, ok := recs[0].(*dns.A); !ok {
		t.Fatalf("expected A record, got %T", recs[0])
	}
}

func TestAAAARecords_Dedup(t *testing.T) {
	b := &mockBackend{
		mockServices: func(_ctx context.Context, _state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			return []msg.Service{
				{Host: "::1", TTL: 60},
				{Host: "::1", TTL: 60},
				{Host: "1.2.3.4", TTL: 60},
			}, nil
		},
	}
	req := new(dns.Msg)
	req.SetQuestion("aaaa.example.org.", dns.TypeAAAA)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	recs, tc, err := AAAA(context.Background(), b, "example.org.", state, nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc {
		t.Fatal("unexpected truncation")
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 AAAA record, got %d", len(recs))
	}
	if _, ok := recs[0].(*dns.AAAA); !ok {
		t.Fatalf("expected AAAA record, got %T", recs[0])
	}
}

func TestTXTWithInternalCNAME(t *testing.T) {
	b := &mockBackend{
		mockServices: func(_ctx context.Context, state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			switch state.QName() {
			case "txt.example.org.":
				return []msg.Service{{Host: "target.example.org.", TTL: 50}}, nil
			case "target.example.org.":
				return []msg.Service{{Text: "v=txt1", TTL: 40}}, nil
			default:
				return nil, nil
			}
		},
	}
	req := new(dns.Msg)
	req.SetQuestion("txt.example.org.", dns.TypeTXT)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	recs, tc, err := TXT(context.Background(), b, "example.org.", state, nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc {
		t.Fatal("unexpected truncation")
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records (CNAME+TXT), got %d", len(recs))
	}
	if _, ok := recs[0].(*dns.CNAME); !ok {
		t.Fatalf("expected first record CNAME, got %T", recs[0])
	}
	if _, ok := recs[1].(*dns.TXT); !ok {
		t.Fatalf("expected second record TXT, got %T", recs[1])
	}
}

func TestCNAMEHostIsNameAndIpIgnored(t *testing.T) {
	b := &mockBackend{
		mockServices: func(_ctx context.Context, _state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			return []msg.Service{
				{Host: "target.example.org.", TTL: 50},
				{Host: "1.2.3.4", TTL: 50},
			}, nil
		},
	}
	req := new(dns.Msg)
	req.SetQuestion("name.example.org.", dns.TypeCNAME)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	recs, err := CNAME(context.Background(), b, "example.org.", state, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 CNAME, got %d", len(recs))
	}
	if _, ok := recs[0].(*dns.CNAME); !ok {
		t.Fatalf("expected CNAME, got %T", recs[0])
	}
}

func TestCNAMEChainLimitAndLoop(t *testing.T) {
	// Construct internal CNAME chain longer than maxCnameChainLength and ensure truncation of chain
	chainLength := maxCnameChainLength + 2
	names := make([]string, 0, chainLength)
	for i := range chainLength {
		names = append(names, fmt.Sprintf("c%d.example.org.", i))
	}
	chain := map[string]string{}
	for i := range len(names) - 1 {
		chain[names[i]] = names[i+1]
	}
	b := &mockBackend{
		mockServices: func(_ctx context.Context, state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			if nxt, ok := chain[state.QName()]; ok {
				return []msg.Service{{Host: nxt, TTL: 10}}, nil
			}
			return []msg.Service{}, nil
		},
	}
	// A query should follow the chain only until limit; since no terminal A, result is 0 or just below limit
	req := new(dns.Msg)
	req.SetQuestion(names[0], dns.TypeA)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	recs, _, err := A(context.Background(), b, "example.org.", state, nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// We cannot exceed limit; if any records exist they must be <= maxCnameChainLength
	if len(recs) > maxCnameChainLength {
		t.Fatalf("CNAME chain exceeded limit: %d > %d", len(recs), maxCnameChainLength)
	}

	// Now create a direct loop: qname CNAME qname should be ignored
	b2 := &mockBackend{
		mockServices: func(_ctx context.Context, state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			return []msg.Service{{Host: state.QName(), TTL: 10}}, nil
		},
	}
	req2 := new(dns.Msg)
	req2.SetQuestion("loop.example.org.", dns.TypeA)
	state2 := request.Request{Req: req2, W: &test.ResponseWriter{}}
	recs2, _, err := A(context.Background(), b2, "example.org.", state2, nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs2) != 0 {
		t.Fatalf("expected 0 records due to CNAME self-loop, got %d", len(recs2))
	}
}

func TestAWithExternalCNAMELookupTruncated(t *testing.T) {
	b := &mockBackend{
		mockServices: func(_ctx context.Context, _state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			return []msg.Service{{Host: "alias.external."}}, nil
		},
		mockLookup: func(_ctx context.Context, _state request.Request, name string, typ uint16) (*dns.Msg, error) {
			if name != "alias.external." || typ != dns.TypeA {
				t.Fatalf("unexpected mockLookup: %s %d", name, typ)
			}
			m := new(dns.Msg)
			m.Truncated = true
			a := &dns.A{Hdr: dns.RR_Header{Name: "alias.external.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 10}, A: net.ParseIP("1.2.3.4")}
			m.Answer = []dns.RR{a}
			return m, nil
		},
	}
	req := new(dns.Msg)
	req.SetQuestion("cname.example.org.", dns.TypeA)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	recs, tc, err := A(context.Background(), b, "example.org.", state, nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tc {
		t.Fatal("expected truncation true")
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records (CNAME+A), got %d", len(recs))
	}
	if _, ok := recs[0].(*dns.CNAME); !ok {
		t.Fatalf("expected first record CNAME, got %T", recs[0])
	}
	if _, ok := recs[1].(*dns.A); !ok {
		t.Fatalf("expected second record A, got %T", recs[1])
	}
}

func TestAAAAWithInternalCNAMEChain(t *testing.T) {
	// Build a short internal chain: n0 -> n1 -> final (AAAA)
	names := []string{"c0.example.org.", "c1.example.org.", "final.example.org."}
	chain := map[string]string{names[0]: names[1], names[1]: names[2]}
	b := &mockBackend{
		mockServices: func(_ctx context.Context, state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			if nxt, ok := chain[state.QName()]; ok {
				return []msg.Service{{Host: nxt, TTL: 10}}, nil
			}
			if state.QName() == names[2] {
				return []msg.Service{{Host: "::1", TTL: 20}}, nil
			}
			return nil, nil
		},
	}
	req := new(dns.Msg)
	req.SetQuestion(names[0], dns.TypeAAAA)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	recs, tc, err := AAAA(context.Background(), b, "example.org.", state, nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc {
		t.Fatal("unexpected truncation")
	}
	if len(recs) != 3 {
		t.Fatalf("expected 2 CNAMEs + 1 AAAA, got %d", len(recs))
	}
	if _, ok := recs[0].(*dns.CNAME); !ok {
		t.Fatalf("expected first record CNAME, got %T", recs[0])
	}
	if _, ok := recs[1].(*dns.CNAME); !ok {
		t.Fatalf("expected second record CNAME, got %T", recs[1])
	}
	if aaaa, ok := recs[2].(*dns.AAAA); !ok || !aaaa.AAAA.Equal(net.ParseIP("::1")) {
		t.Fatalf("expected final AAAA with ::1, got %T %v", recs[2], recs[2])
	}
}

func TestAAAAWithExternalCNAMELookupTruncated(t *testing.T) {
	b := &mockBackend{
		mockServices: func(_ctx context.Context, _state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			return []msg.Service{{Host: "alias.external."}}, nil
		},
		mockLookup: func(_ctx context.Context, _state request.Request, name string, typ uint16) (*dns.Msg, error) {
			if name != "alias.external." || typ != dns.TypeAAAA {
				t.Fatalf("unexpected mockLookup: %s %d", name, typ)
			}
			m := new(dns.Msg)
			m.Truncated = true
			aaaa := &dns.AAAA{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 10}, AAAA: net.ParseIP("::1")}
			m.Answer = []dns.RR{aaaa}
			return m, nil
		},
	}
	req := new(dns.Msg)
	req.SetQuestion("cname6.example.org.", dns.TypeAAAA)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	recs, tc, err := AAAA(context.Background(), b, "example.org.", state, nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tc {
		t.Fatal("expected truncation true")
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records (CNAME+AAAA), got %d", len(recs))
	}
	if _, ok := recs[0].(*dns.CNAME); !ok {
		t.Fatalf("expected first record CNAME, got %T", recs[0])
	}
	if _, ok := recs[1].(*dns.AAAA); !ok {
		t.Fatalf("expected second record AAAA, got %T", recs[1])
	}
}

func TestPTRDomainOnly(t *testing.T) {
	b := &mockBackend{
		mockReverse: func(_ctx context.Context, _state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			return []msg.Service{
				{Host: "name.example.org.", TTL: 20},
				{Host: "1.2.3.4", TTL: 20},
			}, nil
		},
	}
	req := new(dns.Msg)
	req.SetQuestion("4.3.2.1.in-addr.arpa.", dns.TypePTR)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	recs, err := PTR(context.Background(), b, "in-addr.arpa.", state, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 PTR, got %d", len(recs))
	}
	if ptr, ok := recs[0].(*dns.PTR); !ok || ptr.Ptr != "name.example.org." {
		t.Fatalf("unexpected PTR: %T %v", recs[0], recs[0])
	}
}

func TestNSSuccess(t *testing.T) {
	b := &mockBackend{
		mockServices: func(_ctx context.Context, _state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			return []msg.Service{
				{Host: "1.2.3.4", TTL: 30, Key: "/skydns/org/example/ns1"},
				{Host: "::1", TTL: 30, Key: "/skydns/org/example/ns2"},
			}, nil
		},
	}
	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeNS)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	recs, extra, err := NS(context.Background(), b, "example.org.", state, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 NS records, got %d", len(recs))
	}
	if len(extra) != 2 {
		t.Fatalf("expected 2 extra address records, got %d", len(extra))
	}
}

func TestSRVAddressesAndExternalLookup(t *testing.T) {
	// First service is IP host -> produces SRV + extra address; second is CNAME target -> triggers external lookup
	lookedUp := false
	b := &mockBackend{
		mockServices: func(_ctx context.Context, _state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			return []msg.Service{
				{Host: "1.2.3.4", Port: 80, Priority: 10, Weight: 5, TTL: 30, Key: "/skydns/org/example/s1"},
				{Host: "alias.external.", Port: 80, Priority: 10, Weight: 5, TTL: 30, Key: "/skydns/org/example/s2"},
			}, nil
		},
		mockLookup: func(_ctx context.Context, _state request.Request, name string, typ uint16) (*dns.Msg, error) {
			if name == "alias.external." && (typ == dns.TypeA || typ == dns.TypeAAAA) {
				lookedUp = true
				m := new(dns.Msg)
				if typ == dns.TypeA {
					m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 10}, A: net.ParseIP("5.6.7.8")}}
				} else {
					m.Answer = []dns.RR{&dns.AAAA{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 10}, AAAA: net.ParseIP("::1")}}
				}
				return m, nil
			}
			return &dns.Msg{}, nil
		},
	}
	req := new(dns.Msg)
	req.SetQuestion("_sip._tcp.example.org.", dns.TypeSRV)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	recs, extra, err := SRV(context.Background(), b, "example.org.", state, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 SRV records, got %d", len(recs))
	}
	if len(extra) == 0 {
		t.Fatalf("expected extra address records")
	}
	if !lookedUp {
		t.Fatalf("expected external lookup for alias.external.")
	}
}

func TestMXInternalAndExternalTargets(t *testing.T) {
	lookedUp := false
	b := &mockBackend{
		mockServices: func(_ctx context.Context, _state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			return []msg.Service{
				{Host: "1.2.3.4", Mail: true, Priority: 10, TTL: 60, Key: "/skydns/org/example/mx1"},
				{Host: "alias.external.", Mail: true, Priority: 20, TTL: 60, Key: "/skydns/org/example/mx2"},
			}, nil
		},
		mockLookup: func(_ctx context.Context, _state request.Request, name string, typ uint16) (*dns.Msg, error) {
			if name == "alias.external." && (typ == dns.TypeA || typ == dns.TypeAAAA) {
				lookedUp = true
				m := new(dns.Msg)
				if typ == dns.TypeA {
					m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 10}, A: net.ParseIP("9.9.9.9")}}
				} else {
					m.Answer = []dns.RR{&dns.AAAA{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 10}, AAAA: net.ParseIP("::2")}}
				}
				return m, nil
			}
			return &dns.Msg{}, nil
		},
	}
	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeMX)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	recs, extra, err := MX(context.Background(), b, "example.org.", state, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 MX records, got %d", len(recs))
	}
	if len(extra) == 0 {
		t.Fatalf("expected extra address records for MX")
	}
	if !lookedUp {
		t.Fatalf("expected external lookup for alias.external.")
	}
}

func TestSOA(t *testing.T) {
	b := &mockBackend{minTTL: 30, serial: 1234}
	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeSOA)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	recs, err := SOA(context.Background(), b, "example.org.", state, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 SOA, got %d", len(recs))
	}
	soa, ok := recs[0].(*dns.SOA)
	if !ok {
		t.Fatalf("expected SOA, got %T", recs[0])
	}
	if soa.Hdr.Ttl != 30 {
		t.Fatalf("expected ttl 30, got %d", soa.Hdr.Ttl)
	}
	if soa.Ns != "ns.dns.example.org." {
		t.Fatalf("unexpected NS in SOA: %s", soa.Ns)
	}
	if soa.Mbox != "hostmaster.example.org." {
		t.Fatalf("unexpected Mbox in SOA: %s", soa.Mbox)
	}
}

func TestBackendError(t *testing.T) {
	b := &mockBackend{}
	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)
	w := &test.ResponseWriter{}
	state := request.Request{Req: req, W: w}
	code, err := BackendError(context.Background(), b, "example.org.", dns.RcodeServerFailure, state, fmt.Errorf("err"), Options{})
	if code != dns.RcodeSuccess {
		t.Fatalf("expected RcodeSuccess, got %d", code)
	}
	if err == nil {
		t.Fatal("expected error to be returned")
	}
}

func TestCheckForApex(t *testing.T) {
	b := &mockBackend{
		mockServices: func(_ctx context.Context, state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			if state.QName() == "apex.dns.example.org." {
				return []msg.Service{{Host: "1.2.3.4", TTL: 10}}, nil
			}
			return []msg.Service{{Host: "::1", TTL: 20}}, nil
		},
	}
	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	services, err := checkForApex(context.Background(), b, "example.org.", state, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(services) != 1 || services[0].Host != "1.2.3.4" {
		t.Fatalf("expected apex services, got %+v", services)
	}
}

func TestCheckForApexFallback(t *testing.T) {
	errName := errors.New("name not found")
	calls := 0
	b := &mockBackend{
		mockServices: func(_ctx context.Context, state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			calls++
			if state.QName() == "apex.dns.example.org." {
				return nil, errName
			}
			return []msg.Service{{Host: "::1", TTL: 20}}, nil
		},
		mockNameError: func(err error) bool { return errors.Is(err, errName) },
	}
	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	services, err := checkForApex(context.Background(), b, "example.org.", state, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(services) != 1 || services[0].Host != "::1" {
		t.Fatalf("expected fallback services, got %+v", services)
	}
	if calls != 2 {
		t.Fatalf("expected apex and fallback lookups, got %d calls", calls)
	}
	if state.QName() != "example.org." {
		t.Fatalf("query name was not restored: %s", state.QName())
	}
}

func TestCheckForApexFallbackDisabled(t *testing.T) {
	errName := errors.New("name not found")
	calls := 0
	b := &mockBackend{
		mockServices: func(_ctx context.Context, state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			calls++
			if state.QName() != "apex.dns.example.org." {
				t.Fatalf("unexpected fallback lookup for %s", state.QName())
			}
			return nil, errName
		},
		mockNameError: func(err error) bool { return errors.Is(err, errName) },
	}
	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	services, err := checkForApex(context.Background(), b, "example.org.", state, Options{NoApexFallback: true})
	if !errors.Is(err, errName) {
		t.Fatalf("expected name error, got %v", err)
	}
	if len(services) != 0 {
		t.Fatalf("expected no services, got %+v", services)
	}
	if calls != 1 {
		t.Fatalf("expected one apex lookup, got %d calls", calls)
	}
	if state.QName() != "example.org." {
		t.Fatalf("query name was not restored: %s", state.QName())
	}
}

func TestCheckForApexBackendError(t *testing.T) {
	errBackend := context.DeadlineExceeded
	calls := 0
	b := &mockBackend{
		mockServices: func(_ctx context.Context, state request.Request, _exact bool, _opt Options) ([]msg.Service, error) {
			calls++
			if state.QName() != "apex.dns.example.org." {
				t.Fatalf("unexpected fallback lookup for %s", state.QName())
			}
			return nil, errBackend
		},
	}
	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)
	state := request.Request{Req: req, W: &test.ResponseWriter{}}
	services, err := checkForApex(context.Background(), b, "example.org.", state, Options{})
	if !errors.Is(err, errBackend) {
		t.Fatalf("expected backend error, got %v", err)
	}
	if len(services) != 0 {
		t.Fatalf("expected no services, got %+v", services)
	}
	if calls != 1 {
		t.Fatalf("expected one backend lookup, got %d", calls)
	}
	if state.QName() != "example.org." {
		t.Fatalf("query name was not restored: %s", state.QName())
	}
}

func TestIsDuplicate(t *testing.T) {
	m := map[item]struct{}{}
	if isDuplicate(m, "name.", "", 53) {
		t.Fatal("unexpected duplicate on first insert (port)")
	}
	if !isDuplicate(m, "name.", "", 53) {
		t.Fatal("expected duplicate on second insert (port)")
	}
	if isDuplicate(m, "name.", "1.2.3.4", 0) {
		t.Fatal("unexpected duplicate on first insert (addr)")
	}
	if !isDuplicate(m, "name.", "1.2.3.4", 0) {
		t.Fatal("expected duplicate on second insert (addr)")
	}
}

// TestCNAMETargetsChasedOnce checks that a backend returning the same CNAME
// target many times (one per endpoint address, say) causes a single chase of
// that target, both for targets outside the zone (one upstream lookup) and
// for targets inside the zone (one recursive backend lookup).
func TestCNAMETargetsChasedOnce(t *testing.T) {
	const fanout = 50
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeTXT} {
		lookups := 0
		backendCalls := map[string]int{}
		b := &mockBackend{
			mockServices: func(_ context.Context, state request.Request, _ bool, _ Options) ([]msg.Service, error) {
				backendCalls[state.Name()]++
				switch state.Name() {
				case "a.example.org.":
					svcs := make([]msg.Service, 0, 2*fanout)
					for range fanout {
						svcs = append(svcs, msg.Service{Host: "b.example.org", Key: "/skydns/org/example/a"})
						svcs = append(svcs, msg.Service{Host: "ext.example.com", Key: "/skydns/org/example/a"})
					}
					return svcs, nil
				case "b.example.org.":
					if qtype == dns.TypeTXT {
						return []msg.Service{{Text: "hello", Key: "/skydns/org/example/b"}}, nil
					}
					return []msg.Service{{Host: "10.0.0.1", Key: "/skydns/org/example/b"}, {Host: "2001:db8::1", Key: "/skydns/org/example/b"}}, nil
				}
				return nil, errors.New("not found")
			},
			mockLookup: func(_ context.Context, _ request.Request, name string, _ uint16) (*dns.Msg, error) {
				lookups++
				m := new(dns.Msg)
				m.SetQuestion(name, qtype)
				return m, nil
			},
		}

		m := new(dns.Msg)
		m.SetQuestion("a.example.org.", qtype)
		state := request.Request{W: &test.ResponseWriter{}, Req: m, Zone: "example.org."}

		var records []dns.RR
		var err error
		switch qtype {
		case dns.TypeA:
			records, _, err = A(context.TODO(), b, "example.org.", state, nil, Options{})
		case dns.TypeAAAA:
			records, _, err = AAAA(context.TODO(), b, "example.org.", state, nil, Options{})
		case dns.TypeTXT:
			records, _, err = TXT(context.TODO(), b, "example.org.", state, nil, Options{})
		}
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", dns.TypeToString[qtype], err)
		}
		if lookups != 1 {
			t.Errorf("%s: expected 1 upstream lookup for the repeated external target, got %d", dns.TypeToString[qtype], lookups)
		}
		if backendCalls["b.example.org."] != 1 {
			t.Errorf("%s: expected 1 backend lookup for the repeated internal target, got %d", dns.TypeToString[qtype], backendCalls["b.example.org."])
		}
		cnames := 0
		for _, rr := range records {
			if rr.Header().Rrtype == dns.TypeCNAME {
				cnames++
			}
		}
		// One CNAME to b (followed by its data) and one to ext (whose upstream answer is empty here).
		if cnames != 2 {
			t.Errorf("%s: expected 2 CNAME records in the answer, got %d: %v", dns.TypeToString[qtype], cnames, records)
		}
	}
}

func TestCNAMETargetsChasedOnceAcrossRecursion(t *testing.T) {
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeTXT} {
		backendCalls := map[string]int{}
		b := &mockBackend{
			mockServices: func(_ context.Context, state request.Request, _ bool, _ Options) ([]msg.Service, error) {
				backendCalls[state.Name()]++
				switch state.Name() {
				case "a.example.org.":
					return []msg.Service{{Host: "b.example.org"}, {Host: "c.example.org"}}, nil
				case "b.example.org.", "c.example.org.":
					return []msg.Service{{Host: "d.example.org"}}, nil
				case "d.example.org.":
					switch qtype {
					case dns.TypeA:
						return []msg.Service{{Host: "10.0.0.1"}}, nil
					case dns.TypeAAAA:
						return []msg.Service{{Host: "2001:db8::1"}}, nil
					default:
						return []msg.Service{{Text: "hello"}}, nil
					}
				}
				return nil, errors.New("not found")
			},
			mockLookup: func(_ context.Context, _ request.Request, _ string, _ uint16) (*dns.Msg, error) {
				return nil, errors.New("unexpected external lookup")
			},
		}

		m := new(dns.Msg)
		m.SetQuestion("a.example.org.", qtype)
		state := request.Request{W: &test.ResponseWriter{}, Req: m, Zone: "example.org."}

		var err error
		switch qtype {
		case dns.TypeA:
			_, _, err = A(context.Background(), b, "example.org.", state, nil, Options{})
		case dns.TypeAAAA:
			_, _, err = AAAA(context.Background(), b, "example.org.", state, nil, Options{})
		case dns.TypeTXT:
			_, _, err = TXT(context.Background(), b, "example.org.", state, nil, Options{})
		}
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", dns.TypeToString[qtype], err)
		}
		if backendCalls["d.example.org."] != 1 {
			t.Errorf("%s: expected converged target to be chased once, got %d", dns.TypeToString[qtype], backendCalls["d.example.org."])
		}
	}
}

func TestCNAMETargetsAreScopedToBackendEntry(t *testing.T) {
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeTXT} {
		cache := map[string]*dns.Msg{}
		lookupCalls := map[string]int{}
		b := &mockBackend{
			mockServices: func(_ context.Context, state request.Request, _ bool, _ Options) ([]msg.Service, error) {
				switch state.Name() {
				case "a.example.org.":
					return []msg.Service{{Host: "b.example.net"}, {Host: "c.example.net"}}, nil
				case "b.example.net.", "c.example.net.":
					return []msg.Service{{Host: "d.example.org"}}, nil
				case "d.example.org.":
					switch qtype {
					case dns.TypeA:
						return []msg.Service{{Host: "10.0.0.1"}}, nil
					case dns.TypeAAAA:
						return []msg.Service{{Host: "2001:db8::1"}}, nil
					default:
						return []msg.Service{{Text: "hello"}}, nil
					}
				}
				return nil, errors.New("not found")
			},
		}
		b.mockLookup = func(ctx context.Context, _ request.Request, name string, typ uint16) (*dns.Msg, error) {
			lookupCalls[name]++
			if cached := cache[name]; cached != nil {
				return cached.Copy(), nil
			}

			zone := "example.org."
			if dns.IsSubDomain("example.net.", name) {
				zone = "example.net."
			}
			req := new(dns.Msg)
			req.SetQuestion(name, typ)
			state := request.Request{W: &test.ResponseWriter{}, Req: req, Zone: zone}
			var records []dns.RR
			var err error
			switch typ {
			case dns.TypeA:
				records, _, err = A(ctx, b, zone, state, nil, Options{})
			case dns.TypeAAAA:
				records, _, err = AAAA(ctx, b, zone, state, nil, Options{})
			case dns.TypeTXT:
				records, _, err = TXT(ctx, b, zone, state, nil, Options{})
			}
			if err != nil {
				return nil, err
			}
			resp := new(dns.Msg)
			resp.SetReply(req)
			resp.Answer = records
			cache[name] = resp.Copy()
			return resp, nil
		}

		m := new(dns.Msg)
		m.SetQuestion("a.example.org.", qtype)
		state := request.Request{W: &test.ResponseWriter{}, Req: m, Zone: "example.org."}
		var records []dns.RR
		var err error
		switch qtype {
		case dns.TypeA:
			records, _, err = A(context.Background(), b, "example.org.", state, nil, Options{})
		case dns.TypeAAAA:
			records, _, err = AAAA(context.Background(), b, "example.org.", state, nil, Options{})
		case dns.TypeTXT:
			records, _, err = TXT(context.Background(), b, "example.org.", state, nil, Options{})
		}
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", dns.TypeToString[qtype], err)
		}
		if len(records) != 6 {
			t.Fatalf("%s: expected both diamond branches, got %d records: %v", dns.TypeToString[qtype], len(records), records)
		}
		if lookupCalls["d.example.org."] != 2 {
			t.Fatalf("%s: expected each backend entry to chase d, got %d lookups", dns.TypeToString[qtype], lookupCalls["d.example.org."])
		}
		cached := cache["c.example.net."]
		if cached == nil || len(cached.Answer) != 2 {
			t.Fatalf("%s: expected a complete cached answer for c, got %v", dns.TypeToString[qtype], cached)
		}
		if cached.Answer[0].Header().Rrtype != dns.TypeCNAME || cached.Answer[1].Header().Rrtype != qtype {
			t.Errorf("%s: unexpected cached answer for c: %v", dns.TypeToString[qtype], cached.Answer)
		}
	}
}

func TestCNAMECrossZoneLookupBudget(t *testing.T) {
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeTXT} {
		lookups := 0
		b := &mockBackend{
			mockServices: func(_ context.Context, _ request.Request, _ bool, _ Options) ([]msg.Service, error) {
				services := make([]msg.Service, maxCnameLookups+1)
				for i := range services {
					services[i].Host = fmt.Sprintf("target%d.example.net", i)
				}
				return services, nil
			},
			mockLookup: func(_ context.Context, _ request.Request, name string, typ uint16) (*dns.Msg, error) {
				lookups++
				m := new(dns.Msg)
				m.SetQuestion(name, typ)
				return m, nil
			},
		}

		m := new(dns.Msg)
		m.SetQuestion("a.example.org.", qtype)
		state := request.Request{W: &test.ResponseWriter{}, Req: m, Zone: "example.org."}
		var err error
		switch qtype {
		case dns.TypeA:
			_, _, err = A(context.Background(), b, "example.org.", state, nil, Options{})
		case dns.TypeAAAA:
			_, _, err = AAAA(context.Background(), b, "example.org.", state, nil, Options{})
		case dns.TypeTXT:
			_, _, err = TXT(context.Background(), b, "example.org.", state, nil, Options{})
		}
		if err != errMaxCnameLookups {
			t.Fatalf("%s: expected lookup budget error, got %v", dns.TypeToString[qtype], err)
		}
		if lookups != maxCnameLookups {
			t.Errorf("%s: expected %d lookups, got %d", dns.TypeToString[qtype], maxCnameLookups, lookups)
		}
	}
}

func TestCNAMECrossZoneNestedLookupBudget(t *testing.T) {
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeTXT} {
		b := &mockBackend{
			mockServices: func(_ context.Context, state request.Request, _ bool, _ Options) ([]msg.Service, error) {
				if state.Name() == "a.example.org." {
					return []msg.Service{{Host: "nested.example.net"}}, nil
				}
				services := make([]msg.Service, maxCnameLookups)
				for i := range services {
					services[i].Host = fmt.Sprintf("target%d.example.com", i)
				}
				return services, nil
			},
		}
		b.mockLookup = func(ctx context.Context, _ request.Request, name string, typ uint16) (*dns.Msg, error) {
			m := new(dns.Msg)
			m.SetQuestion(name, typ)
			if name != "nested.example.net." {
				return m, nil
			}

			state := request.Request{W: &test.ResponseWriter{}, Req: m, Zone: "example.net."}
			var err error
			switch typ {
			case dns.TypeA:
				_, _, err = A(ctx, b, "example.net.", state, nil, Options{})
			case dns.TypeAAAA:
				_, _, err = AAAA(ctx, b, "example.net.", state, nil, Options{})
			case dns.TypeTXT:
				_, _, err = TXT(ctx, b, "example.net.", state, nil, Options{})
			}
			if err == errMaxCnameLookups {
				m.Rcode = dns.RcodeServerFailure
				return m, nil
			}
			return m, err
		}

		m := new(dns.Msg)
		m.SetQuestion("a.example.org.", qtype)
		state := request.Request{W: &test.ResponseWriter{}, Req: m, Zone: "example.org."}
		var err error
		switch qtype {
		case dns.TypeA:
			_, _, err = A(context.Background(), b, "example.org.", state, nil, Options{})
		case dns.TypeAAAA:
			_, _, err = AAAA(context.Background(), b, "example.org.", state, nil, Options{})
		case dns.TypeTXT:
			_, _, err = TXT(context.Background(), b, "example.org.", state, nil, Options{})
		}
		if err != errMaxCnameLookups {
			t.Fatalf("%s: expected nested lookup budget error, got %v", dns.TypeToString[qtype], err)
		}
	}
}

func TestCNAMECrossZoneLookupBudgetAcrossInZoneRecursion(t *testing.T) {
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeTXT} {
		lookups := 0
		b := &mockBackend{
			mockServices: func(_ context.Context, state request.Request, _ bool, _ Options) ([]msg.Service, error) {
				switch state.Name() {
				case "a.example.org.":
					return []msg.Service{{Host: "b.example.org"}, {Host: "c.example.org"}}, nil
				case "b.example.org.", "c.example.org.":
					start := 0
					count := maxCnameLookups / 2
					if state.Name() == "c.example.org." {
						start = count
						count++
					}
					services := make([]msg.Service, count)
					for i := range services {
						services[i].Host = fmt.Sprintf("target%d.example.net", start+i)
					}
					return services, nil
				default:
					return nil, errors.New("not found")
				}
			},
			mockLookup: func(_ context.Context, _ request.Request, name string, typ uint16) (*dns.Msg, error) {
				lookups++
				m := new(dns.Msg)
				m.SetQuestion(name, typ)
				return m, nil
			},
		}

		m := new(dns.Msg)
		m.SetQuestion("a.example.org.", qtype)
		state := request.Request{W: &test.ResponseWriter{}, Req: m, Zone: "example.org."}
		var err error
		switch qtype {
		case dns.TypeA:
			_, _, err = A(context.Background(), b, "example.org.", state, nil, Options{})
		case dns.TypeAAAA:
			_, _, err = AAAA(context.Background(), b, "example.org.", state, nil, Options{})
		case dns.TypeTXT:
			_, _, err = TXT(context.Background(), b, "example.org.", state, nil, Options{})
		}
		if err != errMaxCnameLookups {
			t.Fatalf("%s: expected lookup budget error, got %v", dns.TypeToString[qtype], err)
		}
		if lookups != maxCnameLookups {
			t.Errorf("%s: expected %d lookups, got %d", dns.TypeToString[qtype], maxCnameLookups, lookups)
		}
	}
}

func TestCNAMECrossZoneLookupBudgetConcurrent(t *testing.T) {
	_, budget := cnameBudget(context.Background())
	accepted := make(chan struct{}, maxCnameLookups*4)
	var wg sync.WaitGroup
	for range maxCnameLookups * 4 {
		wg.Go(func() {
			if budget.take() {
				accepted <- struct{}{}
			}
		})
	}
	wg.Wait()
	if got := len(accepted); got != maxCnameLookups {
		t.Fatalf("expected %d accepted lookups, got %d", maxCnameLookups, got)
	}
}
