package loadbalance

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
)

// weighted round robin specific test data
var testWeighted = []struct {
	expectedWeightFile   string
	expectedWeightReload string
	expectedIsRandom     bool
}{
	{"wfile", "30s", true},
	{"wf", "10s", true},
	{"wf", "0s", false},
}

func TestSetup(t *testing.T) {
	tests := []struct {
		input              string
		shouldErr          bool
		expectedPolicy     string
		expectedErrContent string // substring from the expected error. Empty for positive cases.
		weightedDataIndex  int    // weighted round robin specific data index
	}{
		// positive
		{`loadbalance`, false, "round_robin", "", -1},
		{`loadbalance round_robin`, false, "round_robin", "", -1},
		{`loadbalance weighted_round_robin wfile`, false, "weighted_round_robin", "", 0},
		{`loadbalance weighted_round_robin wf {
                                                reload 10s
                                              } `, false, "weighted_round_robin", "", 1},
		{`loadbalance weighted_round_robin wf {
                                                reload 0s
                                                deterministic
                                              } `, false, "weighted_round_robin", "", 2},
		// negative
		{`loadbalance fleeb`, true, "", "unknown policy", -1},
		{`loadbalance round_robin a`, true, "", "unknown property", -1},
		{`loadbalance weighted_round_robin`, true, "", "missing weight file argument", -1},
		{`loadbalance weighted_round_robin a b`, true, "", "unexpected argument", -1},
		{`loadbalance weighted_round_robin wfile {
                                                   susu
                                                 } `, true, "", "unknown property", -1},
		{`loadbalance weighted_round_robin wfile {
                                                   reload a
                                                 } `, true, "", "invalid reload duration", -1},
		{`loadbalance weighted_round_robin wfile {
                                                    reload 30s  a
                                                 } `, true, "", "unexpected argument", -1},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		policy, w, err := parse(c)

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
		if policy != test.expectedPolicy {
			t.Errorf("Test %d: Expected policy %s but got %s for input %s", i, test.expectedPolicy, policy, test.input)
		}
		if policy == weightedRoundRobinPolicy {
			if err == nil && w == nil {
				t.Errorf("Test %d: Expected valid weight struct but got nil for input %s", i, test.input)
			}
			if err != nil && w != nil {
				t.Errorf("Test %d: Expected nil for weight struct due to error for input %s", i, test.input)
			}
		}
		if policy == weightedRoundRobinPolicy && test.weightedDataIndex >= 0 {
			i := test.weightedDataIndex
			if testWeighted[i].expectedWeightFile != w.fileName {
				t.Errorf("Test %d: Expected weight file name %s but got %s for input %s", i, testWeighted[i].expectedWeightFile, w.fileName, test.input)
			}
			if testWeighted[i].expectedWeightReload != w.reload.String() {
				t.Errorf("Test %d: Expected weight reload duration %s but got %s for input %s", i, testWeighted[i].expectedWeightReload, w.reload, test.input)
			}
			if testWeighted[i].expectedIsRandom != w.isRandom {
				t.Errorf("Test %d: Expected isRandom:%t but got %t for input %s", i, testWeighted[i].expectedIsRandom, w.isRandom, test.input)
			}
		}
	}
}
