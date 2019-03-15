package grpc

import (
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mholt/caddy"
)

func TestSetup(t *testing.T) {
	tests := map[string]struct {
		input                   string
		shouldErr               bool
		expectedFrom            string
		expectedTo              []string
		expectedIgnored         []string
		expectedBackoffMaxDelay time.Duration
		expectedTLSServerName   string
	}{
		"unknown_param":       {"grpc . 127.0.0.1 {\nblaatl\n}\n", true, ".", []string{"127.0.0.1:53"}, nil, 0, ""},
		"wrong_IP":            {"grpc . a27.0.0.1", true, "", nil, nil, 0, ""},
		"ipv6":                {"grpc . [::1]:53", false, ".", []string{"[::1]:53"}, nil, 0, ""},
		"multiple_IPs":        {`grpc . 127.0.0.1 127.0.0.2`, false, ".", []string{"127.0.0.1:53", "127.0.0.2:53"}, nil, 0, ""},
		"multiple_IPs_KO":     {`grpc . 127.0.0.1 127.0.0.2`, true, ".", []string{"127.0.0.1:53", "127.0.0.3:53"}, nil, 0, ""},
		"multiple_IPs_len_KO": {`grpc . 127.0.0.1 127.0.0.2`, true, ".", []string{"127.0.0.1:53"}, nil, 0, ""},
		"from":                {"grpc . 127.0.0.1", false, ".", []string{"127.0.0.1:53"}, nil, 0, ""},
		"form_KO":             {"grpc . 127.0.0.1", true, "127.0.0.1", []string{"127.0.0.1:53"}, nil, 0, ""},
		"except":              {"grpc . 127.0.0.1 {\nexcept miek.nl\n}\n", false, ".", []string{"127.0.0.1:53"}, []string{"miek.nl."}, 0, ""},
		"except_KO":           {"grpc . 127.0.0.1 {\nexcept miek.nl\n}\n", true, ".", []string{"127.0.0.1:53"}, nil, 0, ""},
		"tls":                 {"grpc . 127.0.0.1 {\ntls_servername dns\n}\n", false, ".", []string{"127.0.0.1:53"}, nil, 0, "dns"},
		"tls_KO":              {"grpc . 127.0.0.1 {\ntls_servername dns\n}\n", true, ".", []string{"127.0.0.1:53"}, nil, 0, ""},
		"backoffMaxDelay":     {"grpc . 127.0.0.1 {\nbackoffMaxDelay 2s\n}\n", false, ".", []string{"127.0.0.1:53"}, nil, 2 * time.Second, ""},
		"backoffMaxDelay_KO":  {"grpc . 127.0.0.1 {\nbackoffMaxDelay -2s\n}\n", true, ".", []string{"127.0.0.1:53"}, nil, 0, ""},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			c := caddy.NewTestController("grpc", test.input)
			g, err := parseGRPC(c)

			if err != nil && !test.shouldErr {
				t.Errorf("Test %s: expected error but found %s for input %s", name, err, test.input)
			}

			if err == nil {
				if g.from != test.expectedFrom && !test.shouldErr {
					t.Errorf("Test %s: expected: %s, got: %s", name, test.expectedFrom, g.from)
				}

				if len(g.proxies) != len(test.expectedTo) && !test.shouldErr {
					t.Errorf("Test %s: expected: %s, got: %s", name, test.expectedFrom, g.from)
				}

				var found bool
				for _, proxy := range g.proxies {
					found = false
					for _, to := range test.expectedTo {
						if proxy.addr == to {
							found = true
						}
					}
					if !found && !test.shouldErr {
						t.Errorf("Test %s: proxy %s not found", name, proxy.addr)
					}
				}

				if !reflect.DeepEqual(g.ignored, test.expectedIgnored) && !test.shouldErr {
					t.Errorf("Test %s: expected: %q, actual: %q", name, test.expectedIgnored, g.ignored)
				}
				if g.backoffMaxDelay != test.expectedBackoffMaxDelay && !test.shouldErr {
					t.Errorf("Test %s: expected: %s, got: %s", name, test.expectedBackoffMaxDelay, g.backoffMaxDelay)
				}

				if g.tlsServerName != test.expectedTLSServerName && g.tlsConfig.ServerName != test.expectedTLSServerName && !test.shouldErr {
					t.Errorf("Test %s: expected: %s, got: %s", name, test.expectedTLSServerName, g.tlsServerName)
				}
			}
		})
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
		// pass
		{`grpc . ` + resolv, false, "", []string{"10.10.255.252:53", "10.10.255.253:53"}},
	}

	for i, test := range tests {
		c := caddy.NewTestController("grpc", test.input)
		f, err := parseGRPC(c)

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
				addr := f.proxies[j].addr
				if n != addr {
					t.Errorf("Test %d, expected %q, got %q", j, n, addr)
				}
			}
		}
	}
}
