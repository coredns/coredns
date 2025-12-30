package sign

import (
	"os"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

func TestFileParse(t *testing.T) {
	f, err := os.Open("testdata/db.miek.nl")
	if err != nil {
		t.Fatal(err)
	}
	z, err := Parse(f, "miek.nl.", "testdata/db.miek.nl")
	if err != nil {
		t.Fatal(err)
	}
	s := &Signer{
		directory:  ".",
		signedfile: "db.miek.nl.test",
	}

	s.write(z)
	defer os.Remove("db.miek.nl.test")

	f, err = os.Open("db.miek.nl.test")
	if err != nil {
		t.Fatal(err)
	}
	z, err = Parse(f, "miek.nl.", "db.miek.nl.test")
	if err != nil {
		t.Fatal(err)
	}
	if x := z.Apex.SOA.Header().Name; x != "miek.nl." {
		t.Errorf("Expected SOA name to be %s, got %s", x, "miek.nl.")
	}
	apex, _ := z.Search("miek.nl.")
	key := apex.Type(dns.TypeDNSKEY)
	if key != nil {
		t.Errorf("Expected no DNSKEYs, but got %d", len(key))
	}
}

func TestParseSyntaxErrorBeforeSOA(t *testing.T) {
	const dbSyntaxErrorBeforeSOA = `
$TTL         1M
$ORIGIN      example.org.

@            IN  SOA    ns1.example.com. admin.example.com.  (
                               foobarbaz  ; Invalid serial
                               1200       ; Refresh
                               144        ; Retry
                               1814400    ; Expire
                               2h )       ; Minimum
`
	_, err := Parse(strings.NewReader(dbSyntaxErrorBeforeSOA), "example.org.", "stdin")
	if err == nil {
		t.Fatalf("Zone %q should have failed to load", "example.org.")
	}

	if !strings.Contains(err.Error(), "bad SOA zone parameter") {
		t.Fatalf("Expected parser error, but got: %v", err)
	}
}

func TestParseNoSOA(t *testing.T) {
	const dbNoSOA = `
$TTL         1M
$ORIGIN      example.org.

www          IN  A      192.168.0.14
`
	_, err := Parse(strings.NewReader(dbNoSOA), "example.org.", "stdin")
	if err == nil {
		t.Fatalf("Zone %q should have failed to load", "example.org.")
	}
	if !strings.Contains(err.Error(), "has no SOA record") {
		t.Fatalf("Expected 'no SOA record' error, but got: %v", err)
	}
}
