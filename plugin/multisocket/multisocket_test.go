package multisocket

import (
	"runtime"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

func TestMultisocket(t *testing.T) {
	tests := []struct {
		input              string
		shouldErr          bool
		expectedNumSockets int
		expectedErrContent string // substring from the expected error. Empty for positive cases.
	}{
		// positive
		{`multisocket`, false, runtime.GOMAXPROCS(0), ""},
		{`multisocket 2`, false, 2, ""},
		{`multisocket 1024`, false, 1024, ""},
		{` multisocket 1`, false, 1, ""},
		{`multisocket text`, true, 0, "invalid num sockets"},
		{`multisocket 0`, true, 0, "num sockets can not be zero or negative"},
		{`multisocket -1`, true, 0, "num sockets can not be zero or negative"},
		{`multisocket 1025`, true, 0, "num sockets exceeds maximum"},
		{`multisocket 2 2`, true, 0, "Wrong argument count or unexpected line ending after '2'"},
		{`multisocket 2 {
			block
		}`, true, 0, "Unexpected token '{', expecting argument"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		err := setup(c)
		cfg := dnsserver.GetConfig(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErrContent) {
				t.Errorf("Test %d: Expected error to contain: %v, found error: %v, input: %s", i, test.expectedErrContent, err, test.input)
			}
		}

		if cfg.NumSockets != test.expectedNumSockets {
			t.Errorf("Test %d: Expected num sockets to be %d, found %d", i, test.expectedNumSockets, cfg.NumSockets)
		}
	}
}
