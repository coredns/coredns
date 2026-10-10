package loadbalance

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
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
		{`loadbalance round_robin {
                                                   reload 10s
                                                 } `, true, "", "unknown property", -1},
		{`loadbalance weighted wfile {
                                                   reload
                                                 } `, true, "", "reload duration value is missing", -1},
		{`loadbalance weighted wfile {
                                                   prefer invalid
                                                 } `, true, "", "invalid CIDR", -1},
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

func TestParseWeightedPrefer(t *testing.T) {
	for _, options := range []string{
		"prefer 192.0.2.0/24 2001:db8::/32\nreload 10s",
		"reload 10s\nprefer 192.0.2.0/24 2001:db8::/32",
	} {
		t.Run(options, func(t *testing.T) {
			c := caddy.NewTestController("dns", "loadbalance weighted weights {\n"+options+"\n}")
			lb, err := parse(c)
			if err != nil {
				t.Fatal(err)
			}
			if lb.weighted == nil || lb.weighted.fileName != "weights" || lb.weighted.reload != 10*time.Second {
				t.Fatalf("unexpected weighted configuration: %+v", lb.weighted)
			}
			var got []string
			for _, subnet := range lb.preferSubnets {
				got = append(got, subnet.String())
			}
			want := []string{"192.0.2.0/24", "2001:db8::/32"}
			if !slices.Equal(got, want) {
				t.Fatalf("preferred subnets = %v, want %v", got, want)
			}
		})
	}
}

func TestSetupWeightedPrefer(t *testing.T) {
	c := caddy.NewTestController("dns", `loadbalance weighted weights {
		prefer 192.0.2.0/24
		reload 0s
	}`)
	if err := setup(c); err != nil {
		t.Fatal(err)
	}
	next := plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg).SetReply(r)
		m.Answer = []dns.RR{
			test.A("example.org. 300 IN A 198.51.100.1"),
			test.A("example.org. 300 IN A 192.0.2.1"),
		}
		return dns.RcodeSuccess, w.WriteMsg(m)
	})
	handler := dnsserver.GetConfig(c).Plugin[0](next)
	r := new(dns.Msg)
	r.SetQuestion("example.org.", dns.TypeA)
	w := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := handler.ServeDNS(t.Context(), w, r); err != nil {
		t.Fatal(err)
	}
	if len(w.Msg.Answer) != 2 || w.Msg.Answer[0].(*dns.A).A.String() != "192.0.2.1" {
		t.Fatalf("preferred address is not first in answer: %v", w.Msg.Answer)
	}
}
