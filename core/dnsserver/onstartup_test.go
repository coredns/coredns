package dnsserver

import "testing"

func TestCheckDomainName(t *testing.T) {

	// Note: checkDomainName() returns true for RFC1035 preferred syntax.
	if !checkDomainName("an-example.com.") {
		t.Fatalf(`Expected true but received false for "an-example.com"`)
	}
	if checkDomainName("example-?&^%$.com") {
		t.Fatalf(`Expected false but received true for "example-?&^$.com"`)
	}
}
