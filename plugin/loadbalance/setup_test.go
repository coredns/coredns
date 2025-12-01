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
}{
	{"wfile", "30s"},
	{"wf", "10s"},
	{"wf", "0s"},
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
		{`loadbalance weighted wfile`, false, "weighted", "", 0},
		{`loadbalance weighted wf {
                                                reload 10s
                                              } `, false, "weighted", "", 1},
		{`loadbalance weighted wf {
                                                reload 0s
                                              } `, false, "weighted", "", 2},
		// negative
		{`loadbalance fleeb`, true, "", "unknown policy", -1},
		{`loadbalance round_robin a`, true, "", "unknown property", -1},
		{`loadbalance weighted`, true, "", "missing weight file argument", -1},
		{`loadbalance weighted a b`, true, "", "unexpected argument", -1},
		{`loadbalance weighted wfile {
                                                   susu
                                                 } `, true, "", "unknown property", -1},
		{`loadbalance weighted wfile {
                                                   reload a
                                                 } `, true, "", "invalid reload duration", -1},
		{`loadbalance weighted wfile {
                                                    reload 30s  a
                                                 } `, true, "", "unexpected argument", -1},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		lb, err := parse(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v",
					i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErrContent) {
				t.Errorf("Test %d: Expected error to contain: %v, found error: %v, input: %s",
					i, test.expectedErrContent, err, test.input)
			}
			continue
		}

		if lb == nil {
			t.Errorf("Test %d: Expected valid loadbalance funcs but got nil for input %s",
				i, test.input)
			continue
		}
		policy := ramdomShufflePolicy
		if lb.weighted != nil {
			policy = weightedRoundRobinPolicy
		}
		if policy != test.expectedPolicy {
			t.Errorf("Test %d: Expected policy %s but got %s for input %s", i,
				test.expectedPolicy, policy, test.input)
		}
		if policy == weightedRoundRobinPolicy && test.weightedDataIndex >= 0 {
			i := test.weightedDataIndex
			if testWeighted[i].expectedWeightFile != lb.weighted.fileName {
				t.Errorf("Test %d: Expected weight file name %s but got %s for input %s",
					i, testWeighted[i].expectedWeightFile, lb.weighted.fileName, test.input)
			}
			if testWeighted[i].expectedWeightReload != lb.weighted.reload.String() {
				t.Errorf("Test %d: Expected weight reload duration %s but got %s for input %s",
					i, testWeighted[i].expectedWeightReload, lb.weighted.reload, test.input)
			}
		}
	}
}
