package asnlookup

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

func TestMetadata(t *testing.T) {
	tests := []struct {
		label         string
		expectedValue string
		ipAddress     string
	}{
		{"asnlookup/asn", "15169", "8.8.8.8"},
		{"asnlookup/organization", "GOOGLE", "8.8.8.8"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s/%s", tc.label, "direct"), func(t *testing.T) {
			// Membuat plugin ASNLookup
			asnLookup, err := NewASNLookup("testdata/GeoLite2-ASN.mmdb")
			if err != nil {
				t.Fatalf("unable to create ASNLookup plugin: %v", err)
			}

			state := request.Request{
				Req: new(dns.Msg),
				W:   &test.ResponseWriter{RemoteIP: tc.ipAddress},
			}

			testMetadata(t, state, asnLookup, tc.label, tc.expectedValue)
		})
	}
}

func testMetadata(t *testing.T, state request.Request, asnLookup *ASNLookup, label, expectedValue string) {
	ctx := metadata.ContextWithMetadata(context.Background())
	rCtx := asnLookup.Metadata(ctx, state)
	if fmt.Sprintf("%p", ctx) != fmt.Sprintf("%p", rCtx) {
		t.Errorf("returned context is expected to be the same one passed in the Metadata function")
	}

	fn := metadata.ValueFunc(ctx, label)
	if fn == nil {
		t.Errorf("label %q not set in metadata plugin context", label)
		return
	}

	value := fn()
	if value != expectedValue {
		t.Errorf("expected value for label %q should be %q, got %q instead",
			label, expectedValue, value)
	}
}

type testResponseWriter struct {
	RemoteIP string
}

func (tw *testResponseWriter) WriteMsg(msg *dns.Msg) error {
	return nil
}

func (tw *testResponseWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (tw *testResponseWriter) Close() error {
	return nil
}

func (tw *testResponseWriter) TsigStatus() error {
	return nil
}

func (tw *testResponseWriter) TsigTimersOnly(bool) {}

func (tw *testResponseWriter) Hijack() {}

func (tw *testResponseWriter) LocalAddr() net.Addr {
	return nil
}

func (tw *testResponseWriter) RemoteAddr() net.Addr {
	return nil
}
