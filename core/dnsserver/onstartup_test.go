package dnsserver

import "testing"

func TestCheckDomainName(t *testing.T) {

	// Note: compileRegexp() returns regexp-object for RFC1035 preferred syntax.
	regexpObj := compileRegexp() // regexpObj initialised and declared with a regexp-object for RFC1035 preferred syntax.

	// Below conditions checks for RFC 1035 "preferred syntax"
	if !regexpObj.MatchString(".") {
		t.Fatalf(`Expected true but received false for "." (root domain)`)
	}
	if !regexpObj.MatchString("example.com.") {
		t.Fatalf(`Expected true but received false for "example.com."`)
	}
	if !regexpObj.MatchString("abc-123.com.") {
		t.Fatalf(`Expected true but received false for "abc-123.com."`)
	}
	if !regexpObj.MatchString("abc.123-xyz.") {
		t.Fatalf(`Expected true but received false for "abc.123-xyz."`)
	}
	if !regexpObj.MatchString("an-example.com.") {
		t.Fatalf(`Expected true but received false for "an-example.com."`)
	}

	// Below conditions not checks for RFC 1035 "preferred syntax"
	if regexpObj.MatchString("example-?&^%$.com.") {
		t.Fatalf(`Expected false but received true for "example-?&^$.com."`)
	}
	if regexpObj.MatchString("-example.com.") {
		t.Fatalf(`Expected true but received false for "-example.com."`)
	}
	if regexpObj.MatchString("example.com") {
		t.Fatalf(`Expected true but received false for "example.com"`)
	}
	if regexpObj.MatchString("abc-%$.example.com.") {
		t.Fatalf(`Expected false but received true for "abc-^$.example.com."`)
	}
	if regexpObj.MatchString("abc-.example.com.") {
		t.Fatalf(`Expected false but received true for "abc-.example.com."`)
	}
	if regexpObj.MatchString("123-abc.example.com.") {
		t.Fatalf(`Expected false but received true for "123-abc.example.com."`)
	}
}
