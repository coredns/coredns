package rewrite

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
)

func TestParse(t *testing.T) {
	tests := []struct {
		inputFileRules string
		shouldErr      bool
		errContains    string
	}{
		// parse errors
		{`rewrite`, true, ""},
		{`rewrite name`, true, ""},
		{`rewrite name a.com b.com`, false, ""},
		{`rewrite stop {
    name regex foo bar
    answer name bar foo
}`, false, ""},
		{`rewrite stop name regex foo bar answer name bar foo`, false, ""},
		{`rewrite stop {
    name regex foo bar
    answer name bar foo
    name baz
}`, true, "2 arguments required"},
		{`rewrite stop {
    answer name bar foo
    name regex foo bar
}`, true, "must begin with a name rule"},
		{`rewrite stop`, true, ""},
		{`rewrite continue`, true, ""},
		{`rewrite stop name regex [bad[ bar answer name bar foo`, true, ""},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.inputFileRules)
		_, err := rewriteParse(c)
		if err == nil && test.shouldErr {
			t.Fatalf("Test %d expected errors, but got no error\n---\n%s", i, test.inputFileRules)
		} else if err != nil && !test.shouldErr {
			t.Fatalf("Test %d expected no errors, but got '%v'\n---\n%s", i, err, test.inputFileRules)
		}

		if err != nil && test.errContains != "" && !strings.Contains(err.Error(), test.errContains) {
			t.Errorf("Test %d got wrong error for invalid response rewrite: '%v'\n---\n%s", i, err.Error(), test.inputFileRules)
		}
	}
}
