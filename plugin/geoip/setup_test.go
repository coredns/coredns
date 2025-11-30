package geoip

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

var (
	fixturesDir   = "./testdata"
	cityDBPath    = filepath.Join(fixturesDir, "GeoLite2-City.mmdb")
	asnDBPath     = filepath.Join(fixturesDir, "GeoLite2-ASN.mmdb")
	unknownDBPath = filepath.Join(fixturesDir, "GeoLite2-UnknownDbType.mmdb")
)

func TestProbingIP(t *testing.T) {
	if !probingIP.IsValid() {
		t.Fatalf("Invalid probing IP: %q", probingIP)
	}
}

func TestSetup(t *testing.T) {
	c := caddy.NewTestController("dns", fmt.Sprintf("%s %s", pluginName, cityDBPath))
	plugins := dnsserver.GetConfig(c).Plugin
	if len(plugins) != 0 {
		t.Fatalf("Expected zero plugins after setup, %d found", len(plugins))
	}
	if err := setup(c); err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}
	plugins = dnsserver.GetConfig(c).Plugin
	if len(plugins) != 1 {
		t.Fatalf("Expected one plugin after setup, %d found", len(plugins))
	}
}

func TestGeoIPParse(t *testing.T) {
	c := caddy.NewTestController("dns", fmt.Sprintf("%s %s", pluginName, cityDBPath))
	if err := setup(c); err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}

	tests := []struct {
		shouldErr      bool
		config         string
		expectedErr    string
		expectedDBType int
	}{
		// Valid - City database
		{false, fmt.Sprintf("%s %s\n", pluginName, cityDBPath), "", city},
		{false, fmt.Sprintf("%s %s { edns-subnet }", pluginName, cityDBPath), "", city},
		// Valid - ASN database
		{false, fmt.Sprintf("%s %s\n", pluginName, asnDBPath), "", asn},
		{false, fmt.Sprintf("%s %s { edns-subnet }", pluginName, asnDBPath), "", asn},

		// Invalid
		{true, pluginName, "Wrong argument count", 0},
		{true, fmt.Sprintf("%s %s {\n\tlanguages en fr es zh-CN\n}\n", pluginName, cityDBPath), "unknown property \"languages\"", 0},
		{true, fmt.Sprintf("%s %s\n%s %s\n", pluginName, cityDBPath, pluginName, cityDBPath), "configuring multiple databases is not supported", 0},
		{true, fmt.Sprintf("%s 1 2 3", pluginName), "Wrong argument count", 0},
		{true, fmt.Sprintf("%s { }", pluginName), "Error during parsing", 0},
		{true, fmt.Sprintf("%s /dbpath { city }", pluginName), "unknown property \"city\"", 0},
		{true, fmt.Sprintf("%s /invalidPath\n", pluginName), "failed to open database file: open /invalidPath: no such file or directory", 0},
		{true, fmt.Sprintf("%s %s\n", pluginName, unknownDBPath), "reader does not support the \"UnknownDbType\" database type", 0},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.config)
		geoIP, err := geoipParse(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found none for input %s", i, test.config)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.config, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.config)
			}
			continue
		}

		if geoIP.db.Reader == nil {
			t.Errorf("Test %d: after parsing database reader should be initialized", i)
		}

		if geoIP.db.provides&test.expectedDBType == 0 {
			t.Errorf("Test %d: expected db type %d not found, database file provides %d", i, test.expectedDBType, geoIP.db.provides)
		}
	}
}
