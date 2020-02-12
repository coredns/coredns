package fanout

import (
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/caddyserver/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input           string
		expectedFrom    string
		expectedTo      []string
		expectedIgnored []string
		expectedFails   int
		expectedWorkers int
		expectedPolicy  Policy
		expectedNetwork string
		expectedErr     string
	}{
		//positive
		{input: "fanout . 127.0.0.1", expectedFrom: ".", expectedFails: 2, expectedWorkers: 4, expectedPolicy: FirstPositive, expectedNetwork: "udp"},
		{input: "fanout . 127.0.0.1 {\nexcept a b\nworker-count 3\n}", expectedFrom: ".", expectedFails: 2, expectedWorkers: 3, expectedPolicy: FirstPositive, expectedIgnored: []string{"a.", "b."}, expectedNetwork: "udp"},
		{input: "fanout . 127.0.0.1 {\npolicy any\n}", expectedFrom: ".", expectedFails: 2, expectedWorkers: 4, expectedPolicy: Any, expectedNetwork: "udp"},
		{input: "fanout . 127.0.0.1 127.0.0.2 {\nmax-fail-count 0\nnetwork tcp\n}", expectedFrom: ".", expectedFails: 0, expectedWorkers: 4, expectedPolicy: FirstPositive, expectedNetwork: "tcp", expectedTo: []string{"127.0.0.1:53", "127.0.0.2:53"}},
		//negative
		{input: "fanout . aaa", expectedErr: "not an IP address or file"},
		{input: "fanout . 127.0.0.1 {\nexcept a b\nworker-count 1\n}", expectedErr: "use Forward plugin"},
		{input: "fanout . 127.0.0.1 {\nexcept a b\nworker-count ten\n}", expectedErr: "'ten'"},
		{input: "fanout . 127.0.0.1 {\npolicy mypolicy\n}", expectedErr: "unknown policy"},
		{input: "fanout . 127.0.0.1 127.0.0.2 {\nmax-fail-count 3\nnetwork XXX\n}", expectedErr: "unknown protocol"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		f, err := parseFanout(c)
		if test.expectedErr != "" && err == nil {
			t.Fatalf("Test %d: expected error but not found errors", i)
		}

		if err != nil {
			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Fatalf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
			continue
		}

		if f.from != test.expectedFrom && test.expectedFrom != "" {
			t.Fatalf("Test %d: expected: %s, got: %s", i, test.expectedFrom, f.from)
		}
		if test.expectedIgnored != nil {
			if !reflect.DeepEqual(f.ignored, test.expectedIgnored) {
				t.Fatalf("Test %d: expected: %q, actual: %q", i, test.expectedIgnored, f.ignored)
			}
		}
		if test.expectedTo != nil {
			var to []string
			for i := 0; i < len(f.clients); i++ {
				to = append(to, f.clients[i].Endpoint())
			}
			if !reflect.DeepEqual(to, test.expectedTo) {
				t.Fatalf("Test %d: expected: %q, actual: %q", i, test.expectedTo, to)
			}
		}
		if f.maxFailCount != test.expectedFails {
			t.Fatalf("Test %d: expected: %d, got: %d", i, test.expectedFails, f.maxFailCount)
		}
		if f.workerCount != test.expectedWorkers {
			t.Fatalf("Test %d: expected: %d, got: %d", i, test.expectedWorkers, f.workerCount)
		}
		if f.net != test.expectedNetwork {
			t.Fatalf("Test %d: expected: %v, got: %v", i, test.expectedNetwork, f.net)
		}
		if f.policy != test.expectedPolicy {
			t.Fatalf("Test %d: expected: %v, got: %v", i, test.expectedPolicy, f.policy)
		}
	}
}

func TestSetupResolvconf(t *testing.T) {
	const resolv = "resolv.conf"
	if err := ioutil.WriteFile(resolv,
		[]byte(`nameserver 10.10.255.252
nameserver 10.10.255.253`), 0666); err != nil {
		t.Fatalf("Failed to write resolv.conf file: %s", err)
	}
	defer os.Remove(resolv)

	tests := []struct {
		input         string
		shouldErr     bool
		expectedErr   string
		expectedNames []string
	}{
		{`fanout . ` + resolv, false, "", []string{"10.10.255.252:53", "10.10.255.253:53"}},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		f, err := parseFanout(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
			continue
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if !test.shouldErr {
			for j, n := range test.expectedNames {
				addr := f.clients[j].Endpoint()
				if n != addr {
					t.Errorf("Test %d, expected %q, got %q", j, n, addr)
				}
			}
		}
		for _, p := range f.clients {
			_ = p.Health().Check()
		}
	}
}
