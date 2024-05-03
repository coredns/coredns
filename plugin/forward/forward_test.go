package forward

import (
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin/dnstap"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/pkg/transport"
)

func TestList(t *testing.T) {
	f := Forward{
		proxies: []*proxy.Proxy{
			proxy.NewProxy("TestList", "1.1.1.1:53", transport.DNS),
			proxy.NewProxy("TestList", "2.2.2.2:53", transport.DNS),
			proxy.NewProxy("TestList", "3.3.3.3:53", transport.DNS),
		},
		p: &roundRobin{},
	}

	expect := []*proxy.Proxy{
		proxy.NewProxy("TestList", "2.2.2.2:53", transport.DNS),
		proxy.NewProxy("TestList", "1.1.1.1:53", transport.DNS),
		proxy.NewProxy("TestList", "3.3.3.3:53", transport.DNS),
	}
	got := f.List()

	if len(got) != len(expect) {
		t.Fatalf("Expected: %v results, got: %v", len(expect), len(got))
	}
	for i, p := range got {
		if p.Addr() != expect[i].Addr() {
			t.Fatalf("Expected proxy %v to be '%v', got: '%v'", i, expect[i].Addr(), p.Addr())
		}
	}
}

func TestSetTapPlugin(t *testing.T) {
	for _, tt := range []struct {
		name,
		tapConfig string
		assert func(t *testing.T, src *dnstap.Dnstap, actualTaps []*dnstap.Dnstap)
	}{
		{
			name:      "one dnstap without message_types",
			tapConfig: `dnstap tcp://example.com:6000`,
			assert: func(t *testing.T, src *dnstap.Dnstap, actualTaps []*dnstap.Dnstap) {
				if len(actualTaps) != 1 {
					t.Errorf("Expected: 1 results, got: %v", len(actualTaps))
					return
				}
				if actualTaps[0] != src {
					t.Error("Unexpected dnstap plugin")
				}
			},
		},
		{
			name: "one dnstap with message_types",
			tapConfig: `dnstap tcp://example.com:6000 {
	message_types FORWARDER_QUERY
}`,
			assert: func(t *testing.T, src *dnstap.Dnstap, actualTaps []*dnstap.Dnstap) {
				if len(actualTaps) != 1 {
					t.Errorf("Expected: 1 results, got: %v", len(actualTaps))
					return
				}
				if actualTaps[0] != src {
					t.Error("Unexpected dnstap plugin")
				}
			},
		},
		{
			name: "one dnstap with non forward message_types",
			tapConfig: `dnstap tcp://example.com:6000 {
	message_types CLIENT_RESPONSE
}`,
			assert: func(t *testing.T, src *dnstap.Dnstap, actualTaps []*dnstap.Dnstap) {
				if len(actualTaps) != 0 {
					t.Errorf("Expected: 0 results, got: %v", len(actualTaps))
				}
			},
		},
		{
			name: "two dnstaps without message_types",
			tapConfig: `dnstap /tmp/dnstap.sock full
	dnstap tcp://example.com:6000`,
			assert: func(t *testing.T, src *dnstap.Dnstap, actualTaps []*dnstap.Dnstap) {
				if len(actualTaps) != 2 {
					t.Errorf("Expected: 2 results, got: %v", len(actualTaps))
					return
				}
				if actualTaps[0] != src || src.Next != actualTaps[1] {
					t.Error("Unexpected order of dnstap plugins")
				}
			},
		},
		{
			name: "two dnstaps where one has forward message_types",
			tapConfig: `dnstap /tmp/dnstap.sock full
dnstap tcp://example.com:6000 {
	message_types FORWARDER_QUERY FORWARDER_RESPONSE
}`,
			assert: func(t *testing.T, src *dnstap.Dnstap, actualTaps []*dnstap.Dnstap) {
				if len(actualTaps) != 2 {
					t.Errorf("Expected: 2 results, got: %v", len(actualTaps))
					return
				}
				if actualTaps[0] != src || src.Next != actualTaps[1] {
					t.Error("Unexpected order of dnstap plugins")
				}
			},
		},
		{
			name: "two dnstaps where one has only non-forward message_types",
			tapConfig: `dnstap tcp://example.com:6000 {
	message_types CLIENT_RESPONSE
}
dnstap /tmp/dnstap.sock full`,
			assert: func(t *testing.T, src *dnstap.Dnstap, actualTaps []*dnstap.Dnstap) {
				if len(actualTaps) != 1 {
					t.Errorf("Expected: 1 results, got: %v", len(actualTaps))
					return
				}
				if actualTaps[0] != src.Next {
					t.Error("Unexpected dnstap plugins")
				}
			},
		},
		{
			name: "two dnstaps with only non-forward message types",
			tapConfig: `dnstap tcp://example.com:6000 {
	message_types CLIENT_RESPONSE
}
dnstap /tmp/dnstap.sock full {
	message_types CLIENT_RESPONSE
}`,
			assert: func(t *testing.T, src *dnstap.Dnstap, actualTaps []*dnstap.Dnstap) {
				if len(actualTaps) != 0 {
					t.Errorf("Expected: 0 results, got: %v", len(actualTaps))
				}
			},
		},
		{
			name: "three dnstaps with only one forward message types",
			tapConfig: `dnstap tcp://example.com:6000 {
	message_types CLIENT_RESPONSE
}
dnstap /tmp/dnstap.sock full {
	message_types CLIENT_RESPONSE
}
dnstap tcp://example.com:6000 {
	message_types FORWARDER_QUERY
}`,
			assert: func(t *testing.T, src *dnstap.Dnstap, actualTaps []*dnstap.Dnstap) {
				if len(actualTaps) != 1 {
					t.Errorf("Expected: 1 results, got: %v", len(actualTaps))
					return
				}

				next, ok := src.Next.(*dnstap.Dnstap)
				if !ok {
					t.Errorf("Expected a dnstap plugin")
					return
				}
				if actualTaps[0] != next.Next {
					t.Error("Unexpected order of dnstap plugins")
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tt.tapConfig)
			dnstapSetup, err := caddy.DirectiveAction("dns", "dnstap")
			if err != nil {
				t.Fatal(err)
			}
			if err = dnstapSetup(c); err != nil {
				t.Fatal(err)
			}
			dnsserver.NewServer("", []*dnsserver.Config{dnsserver.GetConfig(c)})
			tap, ok := dnsserver.GetConfig(c).Handler("dnstap").(*dnstap.Dnstap)
			if !ok {
				t.Fatal("Expected a dnstap plugin")
			}
			f := &Forward{}
			f.SetTapPlugin(tap)
			tt.assert(t, tap, f.tapPlugins)
		})
	}
}
