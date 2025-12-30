package file

import (
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
