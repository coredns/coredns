package proxyproto

import (
	"context"
	"encoding/hex"
	"net"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	cproxyproto "github.com/coredns/coredns/plugin/pkg/proxyproto"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	proxyproto "github.com/pires/go-proxyproto"
)

// TestMetadataThroughChain drives metadata.Collect + the chain with the real
// handler as Provider, asserting a downstream plugin reads proxyproto/tlv.
func TestMetadataThroughChain(t *testing.T) {
	tlvs := []proxyproto.TLV{{Type: 0xEA, Value: []byte{0xca, 0xfe}}}
	raw, err := proxyproto.JoinTLVs(tlvs)
	if err != nil {
		t.Fatalf("JoinTLVs: %v", err)
	}
	addr := cproxyproto.NewAddr(
		&net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 53},
		&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 40000},
		raw,
	)
	w := &tlvResponseWriter{addr: addr}

	// Terminal plugin captures what it reads from the metadata label.
	var gotLabel string
	var gotPresent bool
	terminal := plugin.HandlerFunc(func(ctx context.Context, _ dns.ResponseWriter, _ *dns.Msg) (int, error) {
		if f := metadata.ValueFunc(ctx, "proxyproto/tlv"); f != nil {
			gotPresent = true
			gotLabel = f()
		}
		return dns.RcodeSuccess, nil
	})

	// proxyproto handler sits between metadata and the terminal, and is also the
	// Provider the metadata plugin collects from.
	h := handler{Next: terminal}
	meta := metadata.Metadata{
		Zones:     []string{"."},
		Providers: []metadata.Provider{h},
		Next:      h,
	}

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	ctx := meta.Collect(context.Background(), request.Request{W: w, Req: m})
	if _, err := meta.ServeDNS(ctx, w, m); err != nil {
		t.Fatalf("ServeDNS: %v", err)
	}

	if !gotPresent {
		t.Fatal("downstream plugin did not see proxyproto/tlv label")
	}
	if want := hex.EncodeToString(raw); gotLabel != want {
		t.Fatalf("proxyproto/tlv = %q, want %q", gotLabel, want)
	}
}

// nextHandler records that it was called and returns a sentinel rcode.
type nextHandler struct{ called bool }

func (h *nextHandler) Name() string { return "next" }
func (h *nextHandler) ServeDNS(_ context.Context, _ dns.ResponseWriter, _ *dns.Msg) (int, error) {
	h.called = true
	return dns.RcodeSuccess, nil
}

func TestHandlerName(t *testing.T) {
	if got := (handler{}).Name(); got != "proxyproto" {
		t.Fatalf("Name = %q, want %q", got, "proxyproto")
	}
}

func TestHandlerServeDNSPassthrough(t *testing.T) {
	next := &nextHandler{}
	h := handler{Next: next}
	rc, err := h.ServeDNS(context.Background(), &test.ResponseWriter{}, new(dns.Msg))
	if err != nil {
		t.Fatalf("ServeDNS err = %v", err)
	}
	if rc != dns.RcodeSuccess {
		t.Fatalf("rcode = %d, want %d", rc, dns.RcodeSuccess)
	}
	if !next.called {
		t.Fatal("expected next handler to be called")
	}
}

// tlvResponseWriter is a test ResponseWriter whose RemoteAddr returns a
// cproxyproto.Addr carrying the given raw TLV bytes.
type tlvResponseWriter struct {
	test.ResponseWriter
	addr net.Addr
}

func (w *tlvResponseWriter) RemoteAddr() net.Addr { return w.addr }

func TestMetadataPublishesTLV(t *testing.T) {
	// Build raw TLVs the way the listener would capture them.
	tlvs := []proxyproto.TLV{{Type: 0xEA, Value: []byte{0x01, 0x02}}}
	raw, err := proxyproto.JoinTLVs(tlvs)
	if err != nil {
		t.Fatalf("JoinTLVs: %v", err)
	}
	addr := cproxyproto.NewAddr(
		&net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 53},
		&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 40000},
		raw,
	)

	w := &tlvResponseWriter{addr: addr}
	ctx := metadata.ContextWithMetadata(context.Background())
	h := handler{}
	ctx = h.Metadata(ctx, request.Request{W: w, Req: new(dns.Msg)})

	f := metadata.ValueFunc(ctx, "proxyproto/tlv")
	if f == nil {
		t.Fatal("expected proxyproto/tlv metadata to be set")
	}
	if got, want := f(), hex.EncodeToString(raw); got != want {
		t.Fatalf("proxyproto/tlv = %q, want %q", got, want)
	}
}

func TestMetadataNoTLVNoLabel(t *testing.T) {
	// RemoteAddr is a plain addr (not cproxyproto.Addr): no label published.
	w := &tlvResponseWriter{addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1234}}
	ctx := metadata.ContextWithMetadata(context.Background())
	ctx = handler{}.Metadata(ctx, request.Request{W: w, Req: new(dns.Msg)})
	if f := metadata.ValueFunc(ctx, "proxyproto/tlv"); f != nil {
		t.Fatalf("expected no label for plain addr, got %q", f())
	}
}

func TestMetadataAddrWithoutTLVNoLabel(t *testing.T) {
	// cproxyproto.Addr present but carrying no TLVs: no label.
	addr := cproxyproto.NewAddr(
		&net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 53},
		&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 40000},
		nil,
	)
	w := &tlvResponseWriter{addr: addr}
	ctx := metadata.ContextWithMetadata(context.Background())
	ctx = handler{}.Metadata(ctx, request.Request{W: w, Req: new(dns.Msg)})
	if f := metadata.ValueFunc(ctx, "proxyproto/tlv"); f != nil {
		t.Fatalf("expected no label when Addr has no TLVs, got %q", f())
	}
}
