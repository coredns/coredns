package geoip

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
		dbPath        string
		remoteIP      string
	}{
		// City database tests
		{"geoip/city/name", "Cambridge", cityDBPath, "81.2.69.142"},
		{"geoip/country/code", "GB", cityDBPath, "81.2.69.142"},
		{"geoip/country/name", "United Kingdom", cityDBPath, "81.2.69.142"},
		// is_in_european_union is set to true only to work around bool zero value, and test is really being set.
		{"geoip/country/is_in_european_union", "true", cityDBPath, "81.2.69.142"},
		{"geoip/continent/code", "EU", cityDBPath, "81.2.69.142"},
		{"geoip/continent/name", "Europe", cityDBPath, "81.2.69.142"},
		{"geoip/latitude", "52.2242", cityDBPath, "81.2.69.142"},
		{"geoip/longitude", "0.1315", cityDBPath, "81.2.69.142"},
		{"geoip/timezone", "Europe/London", cityDBPath, "81.2.69.142"},
		{"geoip/postalcode", "CB4", cityDBPath, "81.2.69.142"},
		{"geoip/subdivisions/code", "ENG,CAM", cityDBPath, "81.2.69.142"},

		// ASN database tests
		{"geoip/asn/number", "12345", asnDBPath, "81.2.69.142"},
		{"geoip/asn/org", "Test ASN Organization", asnDBPath, "81.2.69.142"},

		// ASN "Not routed" edge case tests (ASN=0)
		// Test data from iptoasn.com where some IP ranges have no assigned ASN.
		{"geoip/asn/number", "0", asnDBPath, "10.0.0.1"},
		{"geoip/asn/org", "Not routed", asnDBPath, "10.0.0.1"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s/%s", tc.label, "direct"), func(t *testing.T) {
			geoIP, err := newGeoIP(tc.dbPath, false)
			if err != nil {
				t.Fatalf("unable to create geoIP plugin: %v", err)
			}
			state := request.Request{
				Req: new(dns.Msg),
				W:   &test.ResponseWriter{RemoteIP: tc.remoteIP},
			}
			testMetadata(t, state, geoIP, tc.label, tc.expectedValue)
		})

		t.Run(fmt.Sprintf("%s/%s", tc.label, "subnet"), func(t *testing.T) {
			geoIP, err := newGeoIP(tc.dbPath, true)
			if err != nil {
				t.Fatalf("unable to create geoIP plugin: %v", err)
			}
			state := request.Request{
				Req: new(dns.Msg),
				W:   &test.ResponseWriter{RemoteIP: "127.0.0.1"},
			}
			state.Req.SetEdns0(4096, false)
			if o := state.Req.IsEdns0(); o != nil {
				addr := net.ParseIP(tc.remoteIP)
				o.Option = append(o.Option, (&dns.EDNS0_SUBNET{
					SourceNetmask: 32,
					Address:       addr,
				}))
			}
			testMetadata(t, state, geoIP, tc.label, tc.expectedValue)
		})
	}
}

func TestMetadataUnknownIP(t *testing.T) {
	// Test that looking up an IP not explicitly in the database doesn't crash.
	// With IncludeReservedNetworks enabled in the test fixture, the geoip2 library
	// returns zero-initialized data rather than an error, so metadata is set with
	// zero values (ASN="0", org="").
	unknownIPAddr := "203.0.113.1" // TEST-NET-3, not explicitly in our fixture.

	geoIP, err := newGeoIP(asnDBPath, false)
	if err != nil {
		t.Fatalf("unable to create geoIP plugin: %v", err)
	}

	state := request.Request{
		Req: new(dns.Msg),
		W:   &test.ResponseWriter{RemoteIP: unknownIPAddr},
	}

	ctx := metadata.ContextWithMetadata(context.Background())
	geoIP.Metadata(ctx, state)

	// For IPs not in the database, geoip2 returns zero values rather than errors.
	// Metadata is set with these zero values.
	fn := metadata.ValueFunc(ctx, "geoip/asn/number")
	if fn == nil {
		t.Errorf("expected metadata to be set for unknown IP")
		return
	}
	if fn() != "0" {
		t.Errorf("expected geoip/asn/number to be \"0\" for unknown IP, got %q", fn())
	}

	fn = metadata.ValueFunc(ctx, "geoip/asn/org")
	if fn == nil {
		t.Errorf("expected metadata to be set for unknown IP")
		return
	}
	if fn() != "" {
		t.Errorf("expected geoip/asn/org to be empty for unknown IP, got %q", fn())
	}
}

func testMetadata(t *testing.T, state request.Request, geoIP *GeoIP, label, expectedValue string) {
	t.Helper()
	ctx := metadata.ContextWithMetadata(context.Background())
	rCtx := geoIP.Metadata(ctx, state)
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
