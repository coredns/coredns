package dnsserver

import (
	"testing"
)

func TestRegex1035PrefSyntax(t *testing.T) {

	// Positive Test cases
	if !checkZoneSyntax(".") {
		t.Fatalf(`Expected true but received false for "."`)
	}
	if !checkZoneSyntax("example.com.") {
		t.Fatalf(`Expected true but received false for "example.com."`)
	}
	if !checkZoneSyntax("abc-123.com.") {
		t.Fatalf(`Expected true but received false for "abc-123.com."`)
	}
	if !checkZoneSyntax("abc.123-xyz.") {
		t.Fatalf(`Expected true but received false for "abc.123-xyz."`)
	}
	if !checkZoneSyntax("an-example.com.") {
		t.Fatalf(`Expected true but received false for "an-example.com."`)
	}

	// Negative Test cases
	if checkZoneSyntax("example-?&^%$.com.") {
		t.Fatalf(`Expected false but received true for "example-?&^$.com."`)
	}
	if checkZoneSyntax("-example.com.") {
		t.Fatalf(`Expected false but received false for "-example.com."`)
	}
	if checkZoneSyntax("example.com") {
		t.Fatalf(`Expected false but received false for "example.com"`)
	}
	if checkZoneSyntax("abc-%$.example.com.") {
		t.Fatalf(`Expected false but received true for "abc-^$.example.com."`)
	}
	if checkZoneSyntax("abc-.example.com.") {
		t.Fatalf(`Expected false but received true for "abc-.example.com."`)
	}
	if checkZoneSyntax("123-abc.example.com.") {
		t.Fatalf(`Expected false but received true for "123-abc.example.com."`)
	}
}
