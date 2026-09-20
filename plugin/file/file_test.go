package file

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkFileParseInsert(b *testing.B) {
	for b.Loop() {
		Parse(strings.NewReader(dbMiekENTNL), testzone, "stdin", 0)
	}
}

func TestParseNoSOA(t *testing.T) {
	_, err := Parse(strings.NewReader(dbNoSOA), "example.org.", "stdin", 0)
	if err == nil {
		t.Fatalf("Zone %q should have failed to load", "example.org.")
	}
	if !strings.Contains(err.Error(), "no SOA record") {
		t.Fatalf("Zone %q should have failed to load with no soa error: %s", "example.org.", err)
	}
}

func TestParseSOAOrigin(t *testing.T) {
	tests := []struct {
		name, origin, prefix, owner string
		wantErr                     bool
	}{
		{name: "at", origin: "example.org.", owner: "@"},
		{name: "absolute", origin: "example.org.", owner: "example.org."},
		{name: "mixed case", origin: "example.org.", owner: "ExAmPlE.OrG."},
		{name: "mixed case origin", origin: "ExAmPlE.OrG.", owner: "example.org."},
		{name: "origin without final dot", origin: "example.org", owner: "@"},
		{name: "relative to parent", origin: "example.org.", prefix: "$ORIGIN org.\n", owner: "example"},
		{name: "decimal escape", origin: "example.org.", owner: `\101xample.org.`},
		{name: "escaped dot", origin: `has\.dot.example.`, owner: `has\046dot.example.`},
		{name: "root", origin: ".", owner: "@"},
		{name: "relative owner", origin: "test", owner: "test", wantErr: true},
		{name: "child", origin: "example.org.", owner: "child.example.org.", wantErr: true},
		{name: "parent", origin: "example.org.", owner: "org.", wantErr: true},
		{name: "unrelated", origin: "example.org.", owner: "example.net.", wantErr: true},
		{name: "different ORIGIN", origin: "example.org.", prefix: "$ORIGIN example.net.\n", owner: "@", wantErr: true},
		{name: "not root", origin: ".", owner: "example.org.", wantErr: true},
	}
	for _, tc := range tests {
		for _, serial := range []int64{-1, 2, 3} {
			t.Run(fmt.Sprintf("%s/serial=%d", tc.name, serial), func(t *testing.T) {
				zone := tc.prefix + tc.owner + " 500 IN SOA ns.example. hostmaster.example. 3 3600 600 86400 300\n"
				z, err := Parse(strings.NewReader(zone), tc.origin, "db.test", serial)
				if tc.wantErr {
					if err == nil {
						t.Fatalf("accepted SOA owner %q for origin %q", z.SOA.Hdr.Name, tc.origin)
					}
					if !strings.Contains(err.Error(), "SOA owner") || !strings.Contains(err.Error(), "db.test") {
						t.Fatalf("expected SOA owner error with file name, got %v", err)
					}
					if z != nil {
						t.Fatal("invalid zone must not be returned")
					}
					return
				}
				if serial == 3 {
					if _, ok := err.(*serialErr); !ok {
						t.Fatalf("expected unchanged serial error, got %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if z.SOA == nil {
					t.Fatal("missing apex SOA")
				}
			})
		}
	}
}

func TestParseLaterSOAOrigin(t *testing.T) {
	zone := `@ 500 IN SOA ns.example. hostmaster.example. 3 3600 600 86400 300
child 500 IN SOA ns.example. hostmaster.example. 4 3600 600 86400 300
`
	z, err := Parse(strings.NewReader(zone), "example.org.", "db.test", -1)
	if err == nil {
		t.Fatalf("replaced apex SOA with %q", z.SOA.Hdr.Name)
	}
	if !strings.Contains(err.Error(), "SOA owner") {
		t.Fatalf("expected SOA owner error, got %v", err)
	}
}

const dbNoSOA = `
$TTL         1M
$ORIGIN      example.org.

www          IN  A      192.168.0.14
mail         IN  A      192.168.0.15
imap         IN  CNAME  mail
`

func TestParseSyntaxError(t *testing.T) {
	_, err := Parse(strings.NewReader(dbSyntaxError), "example.org.", "stdin", 0)
	if err == nil {
		t.Fatalf("Zone %q should have failed to load", "example.org.")
	}
	if !strings.Contains(err.Error(), "\"invalid\"") {
		t.Fatalf("Zone %q should have failed with syntax error: %s", "example.org.", err)
	}
}

const dbSyntaxError = `
$TTL         1M
$ORIGIN      example.org.

@            IN  SOA    ns1.example.com. admin.example.com.  (
                               2005011437 ; Serial
                               1200       ; Refresh
                               144        ; Retry
                               1814400    ; Expire
                               2h )       ; Minimum
@            IN  NS     ns1.example.com.

# invalid comment
www          IN  A      192.168.0.14
mail         IN  A      192.168.0.15
imap         IN  CNAME  mail
`

func TestParseMalformedSOA(t *testing.T) {
	_, err := Parse(strings.NewReader(dbMalformedSOA), "example.org.", "stdin", 0)
	if err == nil {
		t.Fatalf("Zone %q should have failed to load", "example.org.")
	}
	if !strings.Contains(err.Error(), "bad SOA zone parameter") {
		t.Fatalf("Expected parse error containing 'bad SOA zone parameter', got: %s", err)
	}
}

const dbMalformedSOA = `
$TTL         1M
$ORIGIN      example.org.

@            IN  SOA    ns1.example.com. admin.example.com.  (
                               abc ; Serial - invalid
                               1200       ; Refresh
                               144        ; Retry
                               1814400    ; Expire
                               2h )       ; Minimum
@            IN  NS     ns1.example.com.

www          IN  A      192.168.0.14
`

func TestParseSOASerialTooLarge(t *testing.T) {
	_, err := Parse(strings.NewReader(dbSOASerialTooLarge), "example.org.", "stdin", 0)
	if err == nil {
		t.Fatalf("Zone %q should have failed to load", "example.org.")
	}
	if !strings.Contains(err.Error(), "bad SOA zone parameter") {
		t.Fatalf("Expected parse error containing 'bad SOA zone parameter', got: %s", err)
	}
}

const dbSOASerialTooLarge = `
$TTL         1M
$ORIGIN      example.org.

@            IN  SOA    ns1.example.com. admin.example.com.  (
                               202512200817 ; Serial - exceeds 32-bit uint
                               1200       ; Refresh
                               144        ; Retry
                               1814400    ; Expire
                               2h )       ; Minimum
@            IN  NS     ns1.example.com.

www          IN  A      192.168.0.14
`
