package geoip

import (
	"fmt"
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

var (
	_, file, _, _    = runtime.Caller(0)
	fixturesDir      = filepath.Join(filepath.Dir(file), "testdata")
	enterpriseDBPath = filepath.Join(fixturesDir, "GeoIP2-Enterprise.mmdb")
	cityDBPath       = filepath.Join(fixturesDir, "GeoLite2-City.mmdb")
	countryDBPath    = filepath.Join(fixturesDir, "GeoLite2-Country.mmdb")
	unknownDBPath    = filepath.Join(fixturesDir, "GeoLite2-UnknownDbType.mmdb")
	ispDBPath        = filepath.Join(fixturesDir, "GeoIP2-ISP.mmdb")
)

func TestProbingIP(t *testing.T) {
	if probingIP == nil {
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
		expectedLang   []string
		expectedDBType int
	}{
		// Valid
		{false, fmt.Sprintf("%s %s\n", pluginName, enterpriseDBPath), "", defaultLanguages, enterprise},
		{false, fmt.Sprintf("%s %s\n", pluginName, cityDBPath), "", defaultLanguages, city},
		{false, fmt.Sprintf("%s %s\n", pluginName, countryDBPath), "", defaultLanguages, country},
		{false, fmt.Sprintf("%s %s {\n\tlanguages en fr es zh-CN\n}\n", pluginName, cityDBPath), "", []string{"en", "fr", "es", "zh-CN"}, city},
		
		// Invalid
		{true, pluginName, "Wrong argument count", nil, 0},
		{true, fmt.Sprintf("%s %s\n%s %s\n", pluginName, cityDBPath, pluginName, countryDBPath), "configuring multiple databases is not supported", nil, 0},
		{true, fmt.Sprintf("%s 1 2 3", pluginName), "Wrong argument count", nil, 0},
		{true, fmt.Sprintf("%s { }", pluginName), "Error during parsing", nil, 0},
		{true, fmt.Sprintf("%s /dbpath { city }", pluginName), "unknown property", nil, 0},
		{true, fmt.Sprintf("%s /invalidPath\n", pluginName), "failed to open database file: open /invalidPath: no such file or directory", nil, 0},
		{true, fmt.Sprintf("%s %s\n", pluginName, unknownDBPath), "reader does not support the \"UnknownDbType\" database type", defaultLanguages, 0},
		{true, fmt.Sprintf("%s %s\n", pluginName, ispDBPath), "database does not provide city schema", nil, 0},
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
		if len(geoIP.langs) != len(test.expectedLang) {
			t.Errorf("Test %d: plugin languages %v != expected languages %v", i, geoIP.langs, test.expectedLang)
		} else {
			for i, l := range geoIP.langs {
				if test.expectedLang[i] != l {
					t.Errorf("Test %d: plugin language %q != expected language %q", i, l, test.expectedLang[i])
				}
			}
		}
	}

	// Set nil probingIP to test unexpected validate error()
	defer func(ip net.IP) { probingIP = ip }(probingIP)
	probingIP = nil

	c = caddy.NewTestController("dns", fmt.Sprintf("%s %s\n", pluginName, cityDBPath))
	_, err := geoipParse(c)
	if err != nil {
		expectedErr := "unexpected failure looking up database"
		if !strings.Contains(err.Error(), expectedErr) {
			t.Errorf("expected error to contain: %s", expectedErr)
		}
	} else {
		t.Errorf("with a nil probingIP test is expected to fail")
	}
}
