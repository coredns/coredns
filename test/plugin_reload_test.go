package test

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/mholt/caddy"
)

func TestPluginReload(t *testing.T) {
	tests := []struct {
		corefile    string
		corefileNew string
		expected    bool
	}{
		{`.:0 {
			reload 2s
			whoami
			health localhost
		}
			`,
			`.:0 {
			reload 2s
			whoami
			health localhost:0
		}
			`,
			false,
		},
		{`.:0 {
			reload 2s
			whoami
			health localhost:0
		}
			`,
			`.:0 {
			reload 2s
			whoami
			health localhost:0
		}
			`,
			true,
		},
		{`.:0 {
			reload 2s
			whoami
			health localhost:0
		}
			`,
			`.:0 {
			reload 2s
			whoami
			health localhost
		}
			`,
			true,
		},
	}

	for _, tc := range tests {
		actual := reloadCorefile(tc.corefile, tc.corefileNew)
		if actual != tc.expected {
			t.Errorf(
				"failed :\n\texpected: %v\n\t  actual: %v",
				tc.expected,
				actual,
			)
		}
	}
}

func reloadCorefile(corefile, corefileNew string) bool {
	c, _ := CoreDNSServer(corefile)
	c, result := verifyRestart(c, corefileNew)
	c, result = verifyRestart(c, corefile)
	c.Stop()
	return result
}

func verifyRestart(c *caddy.Instance, corefile string) (*caddy.Instance, bool) {
	var buf bytes.Buffer
	result := false

	log.SetOutput(&buf)
	coreInput := NewInput(corefile)
	c, err := c.Restart(coreInput)

	wantMsg := "Running configuration MD5"
	msg := buf.String()
	if err != nil {
		if !strings.Contains(msg, wantMsg) {
			result = false
		}
	}

	if strings.Contains(msg, wantMsg) {
		result = true
	}
	return c, result
}
