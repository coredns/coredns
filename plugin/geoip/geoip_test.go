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
	}{
		{"geoip/city/name", "Cambridge"},

		{"geoip/country/code", "GB"},
		{"geoip/country/name", "United Kingdom"},
		// is_in_european_union is set to true only to work around bool zero value, and test is really being set.
		{"geoip/country/is_in_european_union", "true"},

		{"geoip/continent/code", "EU"},
		{"geoip/continent/name", "Europe"},

		{"geoip/latitude", "52.2242"},
		{"geoip/longitude", "0.1315"},
		{"geoip/timezone", "Europe/London"},
		{"geoip/postalcode", "CB4"},
	}

	knownIPAddr := "81.2.69.142" // This IP should be be part of the CDIR address range used to create the database fixtures.
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s/%s", tc.label, "direct"), func(t *testing.T) {
			geoIP, err := newGeoIP(cityDBPath, false)
			if err != nil {
				t.Fatalf("unable to create geoIP plugin: %v", err)
			}
			state := request.Request{
				Req: new(dns.Msg),
				W:   &test.ResponseWriter{RemoteIP: knownIPAddr},
			}
			testMetadata(t, state, geoIP, tc.label, tc.expectedValue)
		})

		t.Run(fmt.Sprintf("%s/%s", tc.label, "subnet"), func(t *testing.T) {
			geoIP, err := newGeoIP(cityDBPath, true)
			if err != nil {
				t.Fatalf("unable to create geoIP plugin: %v", err)
			}
			state := request.Request{
				Req: new(dns.Msg),
				W:   &test.ResponseWriter{RemoteIP: "127.0.0.1"},
			}
			state.Req.SetEdns0(4096, false)
			if o := state.Req.IsEdns0(); o != nil {
				addr := net.ParseIP(knownIPAddr)
				o.Option = append(o.Option, (&dns.EDNS0_SUBNET{
					SourceNetmask: 32,
					Address:       addr,
				}))
			}
			testMetadata(t, state, geoIP, tc.label, tc.expectedValue)
		})
	}
}

func testMetadata(t *testing.T, state request.Request, geoIP *GeoIP, label, expectedValue string) {
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
