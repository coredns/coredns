package catalog

import (
	"strings"
	"testing"

	"github.com/miekg/dns"
)

func TestParse(t *testing.T) {
	rrs := parseZone(t, `
$ORIGIN catalog.example.
@ 0 IN SOA invalid. hostmaster.invalid. 1 3600 600 604800 0
@ 0 IN NS invalid.
version 0 IN TXT "2"
b.zones 0 IN PTR Example.NET.
a.zones 0 IN PTR example.com.
group.a.zones 0 IN TXT "operator-default"
group.a.zones 0 IN TXT "unsigned"
coo.b.zones 0 IN PTR other-catalog.example.
ignored 0 IN TXT "value"
`)

	cat, err := Parse("catalog.example.", rrs)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cat.Origin != "catalog.example." {
		t.Fatalf("expected origin catalog.example., got %q", cat.Origin)
	}
	if len(cat.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(cat.Members))
	}

	assertMember(t, cat.Members[0], Member{
		ID:     "a",
		Zone:   "example.com.",
		Groups: []string{"operator-default", "unsigned"},
	})
	assertMember(t, cat.Members[1], Member{
		ID:                "b",
		Zone:              "example.net.",
		ChangeOfOwnership: "other-catalog.example.",
	})
}

func TestParseBrokenCatalog(t *testing.T) {
	tests := []struct {
		name string
		zone string
		err  string
	}{
		{
			name: "missing soa",
			zone: `
$ORIGIN catalog.example.
@ 0 IN NS invalid.
version 0 IN TXT "2"
a.zones 0 IN PTR example.com.
`,
			err: "no SOA",
		},
		{
			name: "missing ns",
			zone: `
$ORIGIN catalog.example.
@ 0 IN SOA invalid. hostmaster.invalid. 1 3600 600 604800 0
version 0 IN TXT "2"
a.zones 0 IN PTR example.com.
`,
			err: "no NS",
		},
		{
			name: "missing version",
			zone: `
$ORIGIN catalog.example.
@ 0 IN SOA invalid. hostmaster.invalid. 1 3600 600 604800 0
@ 0 IN NS invalid.
a.zones 0 IN PTR example.com.
`,
			err: "exactly one version",
		},
		{
			name: "multiple version",
			zone: `
$ORIGIN catalog.example.
@ 0 IN SOA invalid. hostmaster.invalid. 1 3600 600 604800 0
@ 0 IN NS invalid.
version 0 IN TXT "2"
version 0 IN TXT "2"
a.zones 0 IN PTR example.com.
`,
			err: "exactly one version",
		},
		{
			name: "unsupported version",
			zone: `
$ORIGIN catalog.example.
@ 0 IN SOA invalid. hostmaster.invalid. 1 3600 600 604800 0
@ 0 IN NS invalid.
version 0 IN TXT "1"
a.zones 0 IN PTR example.com.
`,
			err: "unsupported version",
		},
		{
			name: "multiple member ptr",
			zone: `
$ORIGIN catalog.example.
@ 0 IN SOA invalid. hostmaster.invalid. 1 3600 600 604800 0
@ 0 IN NS invalid.
version 0 IN TXT "2"
a.zones 0 IN PTR example.com.
a.zones 0 IN PTR example.net.
`,
			err: "exactly one PTR",
		},
		{
			name: "duplicate member zone",
			zone: `
$ORIGIN catalog.example.
@ 0 IN SOA invalid. hostmaster.invalid. 1 3600 600 604800 0
@ 0 IN NS invalid.
version 0 IN TXT "2"
a.zones 0 IN PTR example.com.
b.zones 0 IN PTR example.com.
`,
			err: "listed by both",
		},
		{
			name: "multiple coo ptr",
			zone: `
$ORIGIN catalog.example.
@ 0 IN SOA invalid. hostmaster.invalid. 1 3600 600 604800 0
@ 0 IN NS invalid.
version 0 IN TXT "2"
a.zones 0 IN PTR example.com.
coo.a.zones 0 IN PTR new-a.example.
coo.a.zones 0 IN PTR new-b.example.
`,
			err: "more than one coo",
		},
		{
			name: "non in class",
			zone: `
$ORIGIN catalog.example.
@ 0 IN SOA invalid. hostmaster.invalid. 1 3600 600 604800 0
@ 0 IN NS invalid.
version 0 IN TXT "2"
a.zones 0 CH PTR example.com.
`,
			err: "non-IN",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("catalog.example.", parseZone(t, tc.zone))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("expected error containing %q, got %q", tc.err, err)
			}
		})
	}
}

func assertMember(t *testing.T, got, want Member) {
	t.Helper()
	if got.ID != want.ID || got.Zone != want.Zone || got.ChangeOfOwnership != want.ChangeOfOwnership {
		t.Fatalf("expected member %+v, got %+v", want, got)
	}
	if strings.Join(got.Groups, ",") != strings.Join(want.Groups, ",") {
		t.Fatalf("expected groups %v, got %v", want.Groups, got.Groups)
	}
}

func parseZone(t *testing.T, zone string) []dns.RR {
	t.Helper()

	zp := dns.NewZoneParser(strings.NewReader(zone), "catalog.example.", "test")
	var rrs []dns.RR
	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		rrs = append(rrs, rr)
	}
	if err := zp.Err(); err != nil {
		t.Fatalf("failed to parse test zone: %v", err)
	}
	return rrs
}
