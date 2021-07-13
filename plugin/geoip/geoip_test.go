package geoip

import (
	"context"
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
)

func TestMetadata(t *testing.T) {

	tests := []struct {
		dbPath        string
		label         string
		expectedValue string
	}{
		{cityDBPath, "geoip/city/name", "Cambridge"},

		{cityDBPath, "geoip/country/code", "GB"},
		{cityDBPath, "geoip/country/name", "United Kingdom"},
		// is_in_european_union is set to true only to work around bool zero value, and test is really being set.
		{cityDBPath, "geoip/country/is_in_european_union", "true"},

		{cityDBPath, "geoip/continent/code", "EU"},
		{cityDBPath, "geoip/continent/name", "Europe"},

		{cityDBPath, "geoip/latitude", "52.2242"},
		{cityDBPath, "geoip/longitude", "0.1315"},
		{cityDBPath, "geoip/timezone", "Europe/London"},
		{cityDBPath, "geoip/postalcode", "CB4"},
	}

	for i, _test := range tests {
		geoIP, err := newGeoIP(_test.dbPath)
		if err != nil {
			t.Fatalf("Test %d: unable to create geoIP plugin: %v", i, err)
		}
		state := request.Request{
			W: &test.ResponseWriter{RemoteIP: "81.2.69.142"}, // This IP should be be part of the CDIR address range used to create the database fixtures.
		}
		ctx := metadata.ContextWithMetadata(context.Background())
		rCtx := geoIP.Metadata(ctx, state)
		if fmt.Sprintf("%p", ctx) != fmt.Sprintf("%p", rCtx) {
			t.Errorf("Test %d: returned context is expected to be the same one passed in the Metadata function", i)
		}

		fn := metadata.ValueFunc(ctx, _test.label)
		if fn == nil {
			t.Errorf("Test %d: label %q not set in metadata plugin context", i, _test.label)
			continue
		}
		value := fn()
		if value != _test.expectedValue {
			t.Errorf("Test %d: expected value for label %q should be %q, got %q instead",
				i, _test.label, _test.expectedValue, value)
		}
	}
}
