// +build consul

package consul

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/coredns/coredns/middleware/etcd/msg"
	"github.com/coredns/coredns/middleware/pkg/dnsrecorder"
	"github.com/coredns/coredns/middleware/pkg/singleflight"
	"github.com/coredns/coredns/middleware/proxy"
	"github.com/coredns/coredns/middleware/test"
	consulapi "github.com/hashicorp/consul/api"

	"github.com/mholt/caddy"
	"golang.org/x/net/context"
)

func newConsulMiddleware() *Consul {
	ctxt, _ = context.WithTimeout(context.Background(), 300)
	client, _ := newConsulClient("http://127.0.0.1:8500", nil)

	return &Consul{
		Proxy:      proxy.NewLookup([]string{"8.8.8.8:53"}),
		PathPrefix: "skydns",
		Inflight:   &singleflight.Group{},
		Zones:      []string{"skydns.test.", "skydns_extra.test.", "in-addr.arpa."},
		Client:     client,
		ConsulKV:   client.KV(),
	}
}

func set(t *testing.T, e *Consul, k string, m *msg.Service) {
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	path, _ := msg.PathWithWildcard(k, e.PathPrefix)

	// TODO(mia): KV should be a variable, not instantiated every time
	d := &consulapi.KVPair{Key: path[1:], Value: b}
	_, err = e.ConsulKV.Put(d, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func delete(t *testing.T, e *Consul, k string) {
	path, _ := msg.PathWithWildcard(k, e.PathPrefix)
	e.ConsulKV.Delete(path, nil)
}

func TestLookup(t *testing.T) {
	consul := newConsulMiddleware()
	for _, serv := range services {
		set(t, consul, serv.Key, serv)
		defer delete(t, consul, serv.Key)
	}

	for _, tc := range dnsTestCases {
		m := tc.Msg()

		rec := dnsrecorder.New(&test.ResponseWriter{})
		consul.ServeDNS(ctxt, rec, m)

		resp := rec.Msg
		sort.Sort(test.RRSet(resp.Answer))
		sort.Sort(test.RRSet(resp.Ns))
		sort.Sort(test.RRSet(resp.Extra))

		if !test.Header(t, tc, resp) {
			t.Logf("%v\n", resp)
			continue
		}
		if !test.Section(t, tc, test.Answer, resp.Answer) {
			t.Logf("%v\n", resp)
		}
		if !test.Section(t, tc, test.Ns, resp.Ns) {
			t.Logf("%v\n", resp)
		}
		if !test.Section(t, tc, test.Extra, resp.Extra) {
			t.Logf("%v\n", resp)
		}
	}
}

func TestSetupConsul(t *testing.T) {
	tests := []struct {
		input              string
		shouldErr          bool
		expectedPath       string
		expectedEndpoint   string
		expectedErrContent string // substring from the expected error. Empty for positive cases.
	}{
		// positive
		{
			`consul`, false, "coredns", "http://localhost:8500", "",
		},
		{
			`consul skydns.local {
	endpoint localhost:300
}
`, false, "coredns", "localhost:300", "",
		},
		// negative
		{
			`consul {
	endpoints localhost:300
}
`, true, "", "", "unknown property 'endpoints'",
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		consul, _ /*stubzones*/, err := consulParse(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
				continue
			}

			if !strings.Contains(err.Error(), test.expectedErrContent) {
				t.Errorf("Test %d: Expected error to contain: %v, found error: %v, input: %s", i, test.expectedErrContent, err, test.input)
				continue
			}
		}

		if !test.shouldErr && consul.PathPrefix != test.expectedPath {
			t.Errorf("Consul not correctly set for input %s. Expected: %s, actual: %s", test.input, test.expectedPath, consul.PathPrefix)
		}
		if !test.shouldErr && consul.endpoint != test.expectedEndpoint {
			t.Errorf("Consul not correctly set for input %s. Expected: '%s', actual: '%s'", test.input, test.expectedEndpoint, consul.endpoint)
		}
	}
}

var ctxt context.Context
