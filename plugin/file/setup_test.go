package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

const dbRelative = `@ 500 IN SOA ns.example. hostmaster.example. 3 3600 600 86400 300
@ 500 IN NS ns.example.
foo 500 IN A 192.0.2.1
`

func TestFileParse(t *testing.T) {
	zoneFileName1, rm, err := test.TempFile(".", dbMiekNL)
	if err != nil {
		t.Fatal(err)
	}
	defer rm()

	zoneFileName2, rm, err := test.TempFile(".", dbDnssexNLSigned)
	if err != nil {
		t.Fatal(err)
	}
	defer rm()

	zoneFileName3, rm, err := test.TempFile(".", dbRelative)
	if err != nil {
		t.Fatal(err)
	}
	defer rm()

	tests := []struct {
		inputFileRules      string
		shouldErr           bool
		expectedZones       Zones
		expectedFallthrough fall.F
	}{
		{
			`file ` + zoneFileName1 + ` miek.nl.`,
			false,
			Zones{Names: []string{"miek.nl."}},
			fall.Zero,
		},
		{
			`file ` + zoneFileName2 + ` dnssex.nl.`,
			false,
			Zones{Names: []string{"dnssex.nl."}},
			fall.Zero,
		},
		{
			`file ` + zoneFileName3 + ` 10.0.0.0/8`,
			false,
			Zones{Names: []string{"10.in-addr.arpa."}},
			fall.Zero,
		},
		{
			`file ` + zoneFileName3 + ` example.org. {
					fallthrough
				}`,
			false,
			Zones{Names: []string{"example.org."}},
			fall.Root,
		},
		{
			`file ` + zoneFileName3 + ` example.org. {
					fallthrough www.example.org
				}`,
			false,
			Zones{Names: []string{"example.org."}},
			fall.F{Zones: []string{"www.example.org."}},
		},
		// errors.
		{
			`file ` + zoneFileName1 + ` miek.nl {
				transfer from 127.0.0.1
			}`,
			true,
			Zones{},
			fall.Zero,
		},
		{
			`file`,
			true,
			Zones{},
			fall.Zero,
		},
		{
			`file ` + zoneFileName3 + ` example.net. {
				no_reload
			}`,
			true,
			Zones{},
			fall.Zero,
		},
		{
			`file ` + zoneFileName3 + ` example.net. {
				no_rebloat
			}`,
			true,
			Zones{},
			fall.Zero,
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.inputFileRules)
		actualZones, actualFallthrough, err := fileParse(c)

		if err == nil && test.shouldErr {
			t.Fatalf("Test %d expected errors, but got no error", i)
		} else if err != nil && !test.shouldErr {
			t.Fatalf("Test %d expected no errors, but got '%v'", i, err)
		} else {
			if len(actualZones.Names) != len(test.expectedZones.Names) {
				t.Fatalf("Test %d expected %v, got %v", i, test.expectedZones.Names, actualZones.Names)
			}
			for j, name := range test.expectedZones.Names {
				if actualZones.Names[j] != name {
					t.Fatalf("Test %d expected %v for %d th zone, got %v", i, name, j, actualZones.Names[j])
				}
			}
			if !actualFallthrough.Equal(test.expectedFallthrough) {
				t.Errorf("Test %d expected fallthrough of %v, got %v", i, test.expectedFallthrough, actualFallthrough)
			}
		}
	}
}

func TestParseReload(t *testing.T) {
	name, rm, err := test.TempFile(".", dbRelative)
	if err != nil {
		t.Fatal(err)
	}
	defer rm()

	tests := []struct {
		input  string
		reload time.Duration
	}{
		{
			`file ` + name + ` example.org.`,
			1 * time.Minute,
		},
		{
			`file ` + name + ` example.org. {
			reload 5s
			}`,
			5 * time.Second,
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		z, _, err := fileParse(c)
		if err != nil {
			t.Fatal(err)
		}
		if x := z.Z["example.org."].ReloadInterval; x != test.reload {
			t.Errorf("Test %d expected reload to be %s, but got %s", i, test.reload, x)
		}
	}
}

func TestFileParseSOAOrigin(t *testing.T) {
	for _, owner := range []string{"test", "@", "test."} {
		for _, explicit := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/explicit=%t", owner, explicit), func(t *testing.T) {
				contents := fmt.Sprintf(`%s 500 IN SOA ns1.outside.com. root.test 3 604800 86400 2419200 604800
%s 500 IN NS ns1.outside.com.
foo 500 IN A 1.1.1.1
`, owner, owner)
				fileName := filepath.Join(t.TempDir(), "db.test")
				if err := os.WriteFile(fileName, []byte(contents), 0644); err != nil {
					t.Fatal(err)
				}
				corefile := fmt.Sprintf("file %q", filepath.ToSlash(fileName))
				if explicit {
					corefile += " test"
				}
				c := caddy.NewTestController("dns", corefile)
				c.ServerBlockKeys = []string{"test:53"}
				zones, _, err := fileParse(c)
				if owner == "test" {
					if err == nil || !strings.Contains(err.Error(), "SOA owner test.test. that does not match origin test.") {
						t.Fatalf("expected mismatched SOA error, got %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				f := File{Zones: zones}
				for _, tc := range []test.Case{
					{
						Qname: "foo.test.", Qtype: dns.TypeA, Authoritative: true,
						Answer: []dns.RR{test.A("foo.test. 500 IN A 1.1.1.1")},
						Ns:     []dns.RR{test.NS("test. 500 IN NS ns1.outside.com.")},
					},
					{
						Qname: "bar.test.", Qtype: dns.TypeA, Rcode: dns.RcodeNameError, Authoritative: true,
						Ns: []dns.RR{test.SOA("test. 500 IN SOA ns1.outside.com. root.test.test. 3 604800 86400 2419200 604800")},
					},
				} {
					rec := dnstest.NewRecorder(&test.ResponseWriter{})
					if _, err := f.ServeDNS(context.Background(), rec, tc.Msg()); err != nil {
						t.Fatal(err)
					}
					if err := test.SortAndCheck(rec.Msg, tc); err != nil {
						t.Error(err)
					}
				}
			})
		}
	}
}

func TestFileParseRelativeZones(t *testing.T) {
	fileName, rm, err := test.TempFile(".", dbRelative)
	if err != nil {
		t.Fatal(err)
	}
	defer rm()
	c := caddy.NewTestController("dns", "file "+fileName+" example.org example.net")
	zones, _, err := fileParse(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"example.org.", "example.net."} {
		z := zones.Z[origin]
		if z == nil || z.SOA == nil || z.SOA.Hdr.Name != origin {
			t.Fatalf("missing SOA at %s", origin)
		}
		if _, ok := z.Search("foo." + origin); !ok {
			t.Errorf("missing relative A record in %s", origin)
		}
	}
}
