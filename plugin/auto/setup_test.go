package auto

import (
	"testing"
	"time"

	"github.com/coredns/caddy"
)

func TestAutoParse(t *testing.T) {
	tests := []struct {
		inputFileRules         string
		shouldErr              bool
		expectedDirectory      string
		expectedTempl          string
		expectedRe             string
		expectedReloadInterval time.Duration
	}{
		{
			`auto example.org {
				directory /tmp
			}`,
			false, "/tmp", "${1}", `db\.(.*)`, 60 * time.Second,
		},
		{
			`auto 10.0.0.0/24 {
				directory /tmp
			}`,
			false, "/tmp", "${1}", `db\.(.*)`, 60 * time.Second,
		},
		{
			`auto {
				directory /tmp
				reload 0
			}`,
			false, "/tmp", "${1}", `db\.(.*)`, 0 * time.Second,
		},
		{
			`auto {
				directory /tmp (.*) bliep
			}`,
			false, "/tmp", "bliep", `(.*)`, 60 * time.Second,
		},
		{
			`auto {
				directory /tmp (.*) bliep
				reload 10s
			}`,
			false, "/tmp", "bliep", `(.*)`, 10 * time.Second,
		},
		// errors
		// NO_RELOAD has been deprecated.
		{
			`auto {
				directory /tmp
				no_reload
			}`,
			true, "/tmp", "${1}", `db\.(.*)`, 0 * time.Second,
		},
		// TIMEOUT has been deprecated.
		{
			`auto {
				directory /tmp (.*) bliep 10
			}`,
			true, "/tmp", "bliep", `(.*)`, 10 * time.Second,
		},
		// TRANSFER has been deprecated.
		{
			`auto {
				directory /tmp (.*) bliep 10
				transfer to 127.0.0.1
			}`,
			true, "/tmp", "bliep", `(.*)`, 10 * time.Second,
		},
		// no template specified.
		{
			`auto {
				directory /tmp (.*)
			}`,
			true, "/tmp", "", `(.*)`, 60 * time.Second,
		},
		// no directory specified.
		{
			`auto example.org {
				directory
			}`,
			true, "", "${1}", `db\.(.*)`, 60 * time.Second,
		},
		// illegal REGEXP.
		{
			`auto example.org {
				directory /tmp * {1}
			}`,
			true, "/tmp", "${1}", ``, 60 * time.Second,
		},
		// unexpected argument.
		{
			`auto example.org {
				directory /tmp (.*) {1} aa
			}`,
			true, "/tmp", "${1}", ``, 60 * time.Second,
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.inputFileRules)
		a, err := autoParse(c)

		if err == nil && test.shouldErr {
			t.Fatalf("Test %d expected errors, but got no error", i)
		} else if err != nil && !test.shouldErr {
			t.Fatalf("Test %d expected no errors, but got '%v'", i, err)
		} else if !test.shouldErr {
			if a.loader.directory != test.expectedDirectory {
				t.Fatalf("Test %d expected %v, got %v", i, test.expectedDirectory, a.loader.directory)
			}
			if a.loader.template != test.expectedTempl {
				t.Fatalf("Test %d expected %v, got %v", i, test.expectedTempl, a.loader.template)
			}
			if a.loader.re.String() != test.expectedRe {
				t.Fatalf("Test %d expected %v, got %v", i, test.expectedRe, a.loader.re)
			}
			if a.loader.ReloadInterval != test.expectedReloadInterval {
				t.Fatalf("Test %d expected %v, got %v", i, test.expectedReloadInterval, a.loader.ReloadInterval)
			}
		}
	}
}
