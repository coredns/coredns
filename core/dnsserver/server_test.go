package dnsserver

import (
	"context"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/trace"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
)

// testHandler is a net.Listener that implements the net.Listener interface.
type testHandler struct {
}

func (h *testHandler) Accept() (net.Conn, error) {
	return nil, nil
}
func (h *testHandler) Close() error {
	return nil
}
func (h *testHandler) Addr() net.Addr {
	return nil
}

type testPlugin struct{}

func (tp testPlugin) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return 0, nil
}

func (tp testPlugin) Name() string { return "testplugin" }

func testConfig(transport string, p plugin.Handler) *Config {
	c := &Config{
		Zone:        "example.com.",
		Transport:   transport,
		ListenHosts: []string{"127.0.0.1"},
		Port:        "53",
		Debug:       false,
	}

	c.AddPlugin(func(next plugin.Handler) plugin.Handler { return p })
	return c
}

func TestNewServer(t *testing.T) {
	_, err := NewServer("127.0.0.1:53", []*Config{testConfig("dns", testPlugin{})})
	if err != nil {
		t.Errorf("Expected no error for NewServer, got %s", err)
	}

	_, err = NewServergRPC("127.0.0.1:53", []*Config{testConfig("grpc", testPlugin{})})
	if err != nil {
		t.Errorf("Expected no error for NewServergRPC, got %s", err)
	}

	_, err = NewServerTLS("127.0.0.1:53", []*Config{testConfig("tls", testPlugin{})})
	if err != nil {
		t.Errorf("Expected no error for NewServerTLS, got %s", err)
	}
}

func TestDebug(t *testing.T) {
	configNoDebug, configDebug := testConfig("dns", testPlugin{}), testConfig("dns", testPlugin{})
	configDebug.Debug = true

	s1, err := NewServer("127.0.0.1:53", []*Config{configDebug, configNoDebug})
	if err != nil {
		t.Errorf("Expected no error for NewServer, got %s", err)
	}
	if !s1.debug {
		t.Errorf("Expected debug mode enabled for server s1")
	}
	if !log.D.Value() {
		t.Errorf("Expected debug logging enabled")
	}

	s2, err := NewServer("127.0.0.1:53", []*Config{configNoDebug})
	if err != nil {
		t.Errorf("Expected no error for NewServer, got %s", err)
	}
	if s2.debug {
		t.Errorf("Expected debug mode disabled for server s2")
	}
	if log.D.Value() {
		t.Errorf("Expected debug logging disabled")
	}
}

func BenchmarkCoreServeDNS(b *testing.B) {
	s, err := NewServer("127.0.0.1:53", []*Config{testConfig("dns", testPlugin{})})
	if err != nil {
		b.Errorf("Expected no error for NewServer, got %s", err)
	}

	ctx := context.TODO()
	w := &test.ResponseWriter{}
	m := new(dns.Msg)
	m.SetQuestion("aaa.example.com.", dns.TypeTXT)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.ServeDNS(ctx, w, m)
	}
}

func TestServer_WrapListener(t *testing.T) {
	type fields struct {
		Addr         string
		server       [2]*dns.Server
		m            sync.Mutex
		zones        map[string]*Config
		dnsWg        sync.WaitGroup
		graceTimeout time.Duration
		trace        trace.Trace
		debug        bool
		classChaos   bool
	}
	type args struct {
		ln net.Listener
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   net.Listener
	}{
		{
			name: "nil case",
			fields: fields{
				Addr:         ":53",
				server:       [2]*dns.Server{},
				m:            sync.Mutex{},
				zones:        map[string]*Config{},
				dnsWg:        sync.WaitGroup{},
				graceTimeout: time.Second,
				trace:        nil,
				debug:        false,
				classChaos:   false,
			},
			args: args{
				ln: nil,
			},
			want: nil,
		},
		{
			name: "non-nil case",
			fields: fields{
				Addr:         ":53",
				server:       [2]*dns.Server{},
				m:            sync.Mutex{},
				zones:        map[string]*Config{},
				dnsWg:        sync.WaitGroup{},
				graceTimeout: time.Second,
				trace:        nil,
				debug:        false,
				classChaos:   false,
			},
			args: args{
				ln: &testHandler{},
			},
			want: &testHandler{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				Addr:         tt.fields.Addr,
				server:       tt.fields.server,
				m:            tt.fields.m,
				zones:        tt.fields.zones,
				dnsWg:        tt.fields.dnsWg,
				graceTimeout: tt.fields.graceTimeout,
				trace:        tt.fields.trace,
				debug:        tt.fields.debug,
				classChaos:   tt.fields.classChaos,
			}
			if got := s.WrapListener(tt.args.ln); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Server.WrapListener() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServer_Address(t *testing.T) {
	type fields struct {
		Addr         string
		server       [2]*dns.Server
		m            sync.Mutex
		zones        map[string]*Config
		dnsWg        sync.WaitGroup
		graceTimeout time.Duration
		trace        trace.Trace
		debug        bool
		classChaos   bool
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "empty Addr case",
			fields: fields{
				Addr:         "",
				server:       [2]*dns.Server{},
				m:            sync.Mutex{},
				zones:        map[string]*Config{},
				dnsWg:        sync.WaitGroup{},
				graceTimeout: time.Second,
				trace:        nil,
				debug:        false,
				classChaos:   false,
			},
			want: "",
		},
		{
			name: "nil case",
			fields: fields{
				Addr:         ":53",
				server:       [2]*dns.Server{},
				m:            sync.Mutex{},
				zones:        map[string]*Config{},
				dnsWg:        sync.WaitGroup{},
				graceTimeout: time.Second,
				trace:        nil,
				debug:        false,
				classChaos:   false,
			},
			want: ":53",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				Addr:         tt.fields.Addr,
				server:       tt.fields.server,
				m:            tt.fields.m,
				zones:        tt.fields.zones,
				dnsWg:        tt.fields.dnsWg,
				graceTimeout: tt.fields.graceTimeout,
				trace:        tt.fields.trace,
				debug:        tt.fields.debug,
				classChaos:   tt.fields.classChaos,
			}
			if got := s.Address(); got != tt.want {
				t.Errorf("Server.Address() = %v, want %v", got, tt.want)
			}
		})
	}
}
