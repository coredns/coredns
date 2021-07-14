package header

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
)

func TestSetupHeader(t *testing.T) {
	tests := []struct {
		input              string
		shouldErr          bool
		expectedErrContent string
	}{
		{`header {}`, true, "Wrong argument count or unexpected line ending after"},
		{`header {
					set
}`, true, "invalid length for flags, at least one should be provided"},
		{`header {
					foo
}`, true, "invalid length for flags, at least one should be provided"},
		{`header {
					foo bar
}`, true, "unknown flag action=foo, should be set or clear"},
		{`header {
					set ra 
}`, false, ""},
		{`header {
			set ra aa
			clear rd
}`, false, ""},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		err := setup(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found none for input %s", i, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErrContent) {
				t.Errorf("Test %d: Expected error to contain: %v, found error: %v, input: %s", i, test.expectedErrContent, err, test.input)
			}
		}
	}
}
