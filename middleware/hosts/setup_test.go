package hosts

import (
	"testing"

	"github.com/mholt/caddy"
)

func TestHostsParse(t *testing.T) {
	tests := []struct {
		inputFileRules  string
		shouldErr       bool
		expectedPath    string
		expectedOrigins []string
	}{
		{
			`hosts
`,
			false, "/etc/hosts", nil,
		},
		{
			`hosts /tmp`,
			false, "/tmp", nil,
		},
		{
			`hosts /etc/hosts miek.nl.`,
			false, "/etc/hosts", []string{"miek.nl."},
		},
		{
			`hosts /etc/hosts miek.nl. pun.gent.`,
			false, "/etc/hosts", []string{"miek.nl.", "pun.gent."},
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.inputFileRules)
		h, err := hostsParse(c)

		if err == nil && test.shouldErr {
			t.Fatalf("Test %d expected errors, but got no error", i)
		} else if err != nil && !test.shouldErr {
			t.Fatalf("Test %d expected no errors, but got '%v'", i, err)
		} else if !test.shouldErr {
			if h.path != test.expectedPath {
				t.Fatalf("Test %d expected %v, got %v", i, test.expectedPath, h.path)
			}
		} else {
			if len(h.Origins) != len(test.expectedOrigins) {
				t.Fatalf("Test %d expected %v, got %v", i, test.expectedOrigins, h.Origins)
			}
			for j, name := range test.expectedOrigins {
				if h.Origins[j] != name {
					t.Fatalf("Test %d expected %v for %d th zone, got %v", i, name, j, h.Origins[j])
				}
			}
		}
	}
}
