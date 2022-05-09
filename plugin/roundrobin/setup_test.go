package roundrobin

import (
	"reflect"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/roundrobin/internal/strategy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input              string
		shouldErr          bool
		expectedErrContent string // substring from the expected error. Empty for positive cases.
	}{
		{"round_robin ", false, ""},
		{"round_robin stateful", false, ""},
		{"round_robin stateless", false, ""},
		{"round_robin random", false, ""},
		{"round_robin random stateless", false, ""},
		{"round_robin invalid", true, "unknown roundrobin type"},
	}
	for i, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			c := caddy.NewTestController("dns", test.input)
			err := setup(c)

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
		})
	}
}

func TestParseArguments(t *testing.T) {
	var getType = func(v strategy.Shuffler) string {
		if v == nil {
			return ""
		}
		t := reflect.TypeOf(v)
		if t.Kind() == reflect.Ptr {
			return "*" + t.Elem().Name()
		}
		return t.Name()
	}

	tests := []struct {
		input              string
		shouldErr          bool
		expectedStrategy   string
		expectedErrContent string // substring from the expected error. Empty for positive cases.
	}{
		{"round_robin ", false, "*Stateful", ""},
		{"round_robin stateful", false, "*Stateful", ""},
		{"round_robin stateless", false, "*Stateless", ""},
		{"round_robin random", false, "*Random", ""},
		{"round_robin random stateless", false, "*Random", ""},
		{"round_robin invalid", true, "", "unknown roundrobin type"},
	}
	for i, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			c := caddy.NewTestController("dns", test.input)
			strategy, err := parse(c)

			if test.expectedStrategy != getType(strategy) {
				t.Errorf("Test %d: Expected strategy %s but found %s for input %s", i, test.expectedStrategy, getType(strategy), test.input)
			}

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
		})
	}
}
