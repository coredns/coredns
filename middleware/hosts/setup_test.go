package hosts

import (
	"testing"

	"github.com/mholt/caddy"
)

func TestHostsParse(t *testing.T) {
	tests := []struct {
		inputFileRules string
		shouldErr      bool
		expectedPath   string
	}{
		{
			`hosts {
			}`,
			false, "/etc/hosts",
		},
		{
			`hosts {
				file /tmp
			}`,
			false, "/tmp",
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.inputFileRules)
		a, err := hostsParse(c)

		if err == nil && test.shouldErr {
			t.Fatalf("Test %d expected errors, but got no error", i)
		} else if err != nil && !test.shouldErr {
			t.Fatalf("Test %d expected no errors, but got '%v'", i, err)
		} else if !test.shouldErr {
			if a.path != test.expectedPath {
				t.Fatalf("Test %d expected %v, got %v", i, test.expectedPath, a.path)
			}
		}
	}
}
