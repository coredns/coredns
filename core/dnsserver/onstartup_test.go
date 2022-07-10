package dnsserver

import (
	"testing"
)

func TestRegex1035PrefSyntax(t *testing.T) {
	testCases := []struct {
		zone     string
		expected bool
	}{
		{zone: ".", expected: true},
		{zone: "example.com.", expected: true},
		{zone: "example.", expected: true},
		{zone: "example123.", expected: true},
		{zone: "example123.com.", expected: true},
		{zone: "abc-123.com.", expected: true},
		{zone: "an-example.com.", expected: true},
		{zone: "a.example.com.", expected: true},
		{zone: "1.0.0.2.ip6.arpa.", expected: true},
		{zone: "0.10.in-addr.arpa.", expected: true},
		{zone: "example", expected: false},
		{zone: "example:.", expected: false},
		{zone: "-example.com.", expected: false},
		{zone: ".example.com.", expected: false},
		{zone: "1.example.com", expected: false},
		{zone: "abc.123-xyz.", expected: false},
		{zone: "example-?&^%$.com.", expected: false},
		{zone: "abc-.example.com.", expected: false},
		{zone: "abc-%$.example.com.", expected: false},
		{zone: "123-abc.example.com.", expected: false},
	}

	for _, testCase := range testCases {
		if checkZoneSyntax(testCase.zone) != testCase.expected {
			t.Errorf("Expected %v for %q", testCase.expected, testCase.zone)
		}
	}
}
