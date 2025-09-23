package nomad

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	nomad "github.com/hashicorp/nomad/api"
	"github.com/miekg/dns"
)

func TestNomad(t *testing.T) {
	var demoNomad = Nomad{
		Next:    test.ErrorHandler(),
		ttl:     uint32(defaultTTL),
		Zone:    "service.nomad",
		clients: make([]*nomad.Client, 0),
	}

	var cases = []test.Case{
		{
			Qname: "example.default.service.nomad.",
			Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.A("example.default.service.nomad.	30	IN	A	1.2.3.4"),
			},
		},
		{
			Qname:  "example.default.service.nomad.",
			Qtype:  dns.TypeAAAA,
			Rcode:  dns.RcodeSuccess,
			Answer: []dns.RR{},
		},
		{
			Qname: "fakeipv6.default.service.nomad.",
			Qtype: dns.TypeAAAA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.AAAA("fakeipv6.default.service.nomad.	30	IN	AAAA	1:2:3::4"),
			},
		},
		{
			Qname:  "fakeipv6.default.service.nomad.",
			Qtype:  dns.TypeA,
			Rcode:  dns.RcodeSuccess,
			Answer: []dns.RR{},
		},
		{
			Qname: "multi.default.service.nomad.",
			Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.A("multi.default.service.nomad.	30	IN	A	1.2.3.4"),
				test.A("multi.default.service.nomad.	30	IN	A	1.2.3.5"),
				test.A("multi.default.service.nomad.	30	IN	A	1.2.3.6"),
			},
		},
		{
			Qname: "nonexistent.default.service.nomad.",
			Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
			Answer: []dns.RR{
				test.SOA("nonexistent.default.service.nomad.	30	IN	SOA	ns1.nonexistent.default.service.nomad. hostmaster.service.nomad. 0 3600 600 86400 30"),
			},
		},
		{
			Qname:  "example.default.service.nomad.",
			Qtype:  dns.TypeSRV,
			Rcode:  dns.RcodeSuccess,
			Answer: []dns.RR{test.SRV("example.default.service.nomad.	30	IN	SRV	10 10 23202 example.default.service.nomad.")},
			Extra:  []dns.RR{test.A("example.default.service.nomad.  30       IN      A       1.2.3.4")},
		},
	}

	ctx := context.Background()

	// Setup a fake Nomad servers.
	nomadServer1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/service/example":
			w.Write([]byte(`[{"Address":"1.2.3.4","Namespace":"default","Port":23202,"ServiceName":"example"}]`))
		case "/v1/service/fakeipv6":
			w.Write([]byte(`[{"Address":"1:2:3::4","Namespace":"default","Port":8000,"ServiceName":"fakeipv6"}]`))
		case "/v1/service/multi":
			w.Write([]byte(`[{"Address":"1.2.3.4","Namespace":"default","Port":25395,"ServiceName":"multi"},{"Address":"1.2.3.5","Namespace":"default","Port":20888,"ServiceName":"multi"},{"Address":"1.2.3.6","Namespace":"default","Port":26292,"ServiceName":"multi"}]`))
		case "/v1/service/nonexistent":
			w.Write([]byte(`[]`))
		case "/v1/agent/self":
			w.Write([]byte(`{"Member":{"Name":"foobar1"}}`))
		}
	}))
	defer nomadServer1.Close()
	nomadServer2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/service/example":
			w.Write([]byte(`[{"Address":"1.2.3.4","Namespace":"default","Port":23202,"ServiceName":"example"}]`))
		case "/v1/service/fakeipv6":
			w.Write([]byte(`[{"Address":"1:2:3::4","Namespace":"default","Port":8000,"ServiceName":"fakeipv6"}]`))
		case "/v1/service/multi":
			w.Write([]byte(`[{"Address":"1.2.3.4","Namespace":"default","Port":25395,"ServiceName":"multi"},{"Address":"1.2.3.5","Namespace":"default","Port":20888,"ServiceName":"multi"},{"Address":"1.2.3.6","Namespace":"default","Port":26292,"ServiceName":"multi"}]`))
		case "/v1/service/nonexistent":
			w.Write([]byte(`[]`))
		case "/v1/agent/self":
			w.Write([]byte(`{"Member":{"Name":"foobar2"}}`))
		}
	}))
	defer nomadServer2.Close()
	nomadServer3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/service/example":
			w.Write([]byte(`[{"Address":"1.2.3.4","Namespace":"default","Port":23202,"ServiceName":"example"}]`))
		case "/v1/service/fakeipv6":
			w.Write([]byte(`[{"Address":"1:2:3::4","Namespace":"default","Port":8000,"ServiceName":"fakeipv6"}]`))
		case "/v1/service/multi":
			w.Write([]byte(`[{"Address":"1.2.3.4","Namespace":"default","Port":25395,"ServiceName":"multi"},{"Address":"1.2.3.5","Namespace":"default","Port":20888,"ServiceName":"multi"},{"Address":"1.2.3.6","Namespace":"default","Port":26292,"ServiceName":"multi"}]`))
		case "/v1/service/nonexistent":
			w.Write([]byte(`[]`))
		case "/v1/agent/self":
			w.Write([]byte(`{"Member":{"Name":"foobar3"}}`))
		}
	}))
	defer nomadServer3.Close()

	// Configure the plugin to use the fake Nomad server.
	cfg := nomad.DefaultConfig()
	cfg.Address = nomadServer1.URL
	client1, err := nomad.NewClient(cfg)
	if err != nil {
		t.Errorf("Failed to create Nomad client: %v", err)
		return
	}

	demoNomad.clients = append(demoNomad.clients, client1)

	cfg = nomad.DefaultConfig()
	cfg.Address = nomadServer2.URL
	client2, err := nomad.NewClient(cfg)
	if err != nil {
		t.Errorf("Failed to create Nomad client: %v", err)
		return
	}

	demoNomad.clients = append(demoNomad.clients, client2)

	cfg = nomad.DefaultConfig()
	cfg.Address = nomadServer3.URL
	client3, err := nomad.NewClient(cfg)
	if err != nil {
		t.Errorf("Failed to create Nomad client: %v", err)
		return
	}
	demoNomad.clients = append(demoNomad.clients, client3)

	client, _ := demoNomad.getClient()
	if client != client1 {
		t.Errorf("Expected client1")
	}

	nomadServer1.Close()

	client, _ = demoNomad.getClient()
	if client == client1 {
		t.Errorf("Expected client2")
	}

	client, err = demoNomad.getClient()
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}
	if client == nil {
		t.Errorf("Expected client2 but got nil")
	}
	if client != client2 {
		t.Errorf("Expected client2")
	}

	runTests(ctx, t, &demoNomad, cases)
}

func runTests(ctx context.Context, t *testing.T, n *Nomad, cases []test.Case) {
	t.Helper()
	for i, tc := range cases {
		r := tc.Msg()
		w := dnstest.NewRecorder(&test.ResponseWriter{})

		_, err := n.ServeDNS(ctx, w, r)
		if err != tc.Error {
			t.Errorf("Test %d: %v (%v) (%v)", i, err, w, r)
			return
		}

		if w.Msg == nil {
			t.Errorf("Test %d: nil message", i)
		}
		if err := test.SortAndCheck(w.Msg, tc); err != nil {
			t.Errorf("Test %d: %v", i, err)
		}
	}
}
