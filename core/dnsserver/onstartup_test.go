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
	if !checkZoneSyntax("example.") {
		t.Fatalf(`Expected true but received false for "example.com."`)
	}
	if !checkZoneSyntax("example123.") {
		t.Fatalf(`Expected true but received false for "example123."`)
	}
	if !checkZoneSyntax("example123.com.") {
		t.Fatalf(`Expected true but received false for "example123."`)
	}
	if !checkZoneSyntax("abc-123.com.") {
		t.Fatalf(`Expected true but received false for "abc-123.com."`)
	}
	if !checkZoneSyntax("an-example.com.") {
		t.Fatalf(`Expected true but received false for "an-example.com."`)
	}
	if !checkZoneSyntax("an.example.com.") {
		t.Fatalf(`Expected true but received false for "an.example.com."`)
	}

	// Negative Test cases
	if !checkZoneSyntax("1a.example.com") {
		t.Fatalf(`Expected false but received true for "example:."`)
	}
	if !checkZoneSyntax("example:.") {
		t.Fatalf(`Expected false but received true for "example:."`)
	}
	if !checkZoneSyntax("abc.123-xyz.") {
		t.Fatalf(`Expected false but received true for "abc.123-xyz."`)
	}
	if checkZoneSyntax("example-?&^%$.com.") {
		t.Fatalf(`Expected false but received true for "example-?&^$.com."`)
	}
	if checkZoneSyntax("-example.com.") {
		t.Fatalf(`Expected false but received true for "-example.com."`)
	}
	if checkZoneSyntax("example.com") {
		t.Fatalf(`Expected false but received true for "example.com"`)
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
