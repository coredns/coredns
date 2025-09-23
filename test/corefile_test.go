package test

import (
	"testing"
)

// TestCorefileParsing tests the Corefile parsing functionality.
// Expected to not panic or timeout.
func TestCorefileParsing(t *testing.T) {
	cases := []struct {
		name     string
		corefile string
	}{
		{
			// See: https://github.com/coredns/coredns/pull/4637
			name: "PR4637_" + "NoPanicOnEscapedBackslashesAndUnicode",
			corefile: `\\\\ȶ.
acl
`,
		},
		{
			// See: https://github.com/coredns/coredns/pull/7571
			name: "PR7571_" + "InvalidBlockFailsToStart",
			corefile: "\xD9//\n" +
				"hosts#\x90\xD0{lc\x0C{\n" +
				"'{mport\xEF1\x0C}\x0B''",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Expected no panic, but got %v", r)
				}
			}()

			i, _, _, _ := CoreDNSServerAndPorts(tc.corefile)

			defer func() {
				if i != nil {
					i.Stop()
				}
			}()
		})
	}
}
