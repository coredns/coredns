package auto

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/coredns/caddy"
)

func TestAutoParse(t *testing.T) {
	t.Parallel()
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
		// non-existent directory.
		{
			`auto example.org {
				directory /foobar/coredns * {1}
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
		// upstream directive should not error and should consume args
		{
			`auto example.org {
				directory /tmp
				upstream 8.8.8.8 1.1.1.1
			}`,
			false, "/tmp", "${1}", `db\.(.*)`, 60 * time.Second,
		},
		// upstream directive with no args should not error
		{
			`auto example.org {
				directory /tmp
				upstream
			}`,
			false, "/tmp", "${1}", `db\.(.*)`, 60 * time.Second,
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("test_%d", i), func(t *testing.T) {
			t.Parallel()
			c := caddy.NewTestController("dns", test.inputFileRules)
			a, err := autoParse(c)

			if err == nil && test.shouldErr {
				t.Fatalf("Test %d expected errors, but got no error", i)
			} else if err != nil && !test.shouldErr {
				t.Fatalf("Test %d expected no errors, but got '%v'", i, err)
			} else if !test.shouldErr {
				if a.directory != test.expectedDirectory {
					t.Fatalf("Test %d expected %v, got %v", i, test.expectedDirectory, a.directory)
				}
				if a.template != test.expectedTempl {
					t.Fatalf("Test %d expected %v, got %v", i, test.expectedTempl, a.template)
				}
				if a.re.String() != test.expectedRe {
					t.Fatalf("Test %d expected %v, got %v", i, test.expectedRe, a.re)
				}
				if a.ReloadInterval != test.expectedReloadInterval {
					t.Fatalf("Test %d expected %v, got %v", i, test.expectedReloadInterval, a.ReloadInterval)
				}
			}
		})
	}
}

func TestSetupReload(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{
			name: "reload valid",
			config: `auto {
				directory .
				reload 5s
			}`,
			wantErr: false,
		},
		{
			name: "reload disable",
			config: `auto {
				directory .
				reload 0
			}`,
			wantErr: false,
		},
		{
			name: "reload invalid",
			config: `auto {
				directory .
				reload -1s
			}`,
			wantErr: true,
		},
		{
			name: "reload invalid",
			config: `auto {
				directory .
				reload
			}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctr := caddy.NewTestController("dns", tt.config)
			if err := setup(ctr); (err != nil) != tt.wantErr {
				t.Errorf("Error: setup() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAutoParseLargeRegex(t *testing.T) {
	largeRegex := strings.Repeat("a", maxRegexpLen+1)
	config := fmt.Sprintf(`auto {
        directory /tmp %s {1}
    }`, largeRegex)

	c := caddy.NewTestController("dns", config)
	_, err := autoParse(c)
	if err == nil {
		t.Fatal("Expected error for large regex, got nil")
	}
	if !strings.Contains(err.Error(), "regexp too large") {
		t.Errorf("Expected 'regexp too large' error, got: %v", err)
	}
}
