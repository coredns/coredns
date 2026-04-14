package dso

import (
	"slices"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/google/go-cmp/cmp"
	"github.com/miekg/dns"
)

func TestConfigParseUnknown(t *testing.T) {
	c := caddy.NewTestController("dns", "dso {\nunknown\n}")
	_, err := parseConfig(c)
	if err == nil {
		t.Error("Want error")
	}
}

func TestConfigParseTCPPort(t *testing.T) {
	for _, test := range []struct {
		config  string
		port    int
		wantErr bool
	}{
		{"dso {\ntcp_port\n}", 0, true},
		{"dso {\ntcp_port 1 2\n}", 0, true},
		{"dso {\ntcp_port 65536\n}", 0, true},
		{"dso {\ntcp_port -1\n}", -1, true},
		{"dso {\ntcp_port 65535\n}", 65535, false},
	} {
		t.Run(test.config, func(t *testing.T) {
			c := caddy.NewTestController("dns", test.config)
			cfg, err := parseConfig(c)
			switch {
			case test.wantErr && err == nil:
				t.Error("Want error")
			case !test.wantErr && err != nil:
				t.Errorf("Got %v", err)
			case err == nil:
				if cfg.TCPPort != test.port {
					t.Errorf("Got TCPPort=%v, want %v", cfg.TCPPort, test.port)
				}
			}
		})
	}
}

func TestConfigParseTLSPort(t *testing.T) {
	for _, test := range []struct {
		config  string
		port    int
		wantErr bool
	}{
		{"dso {\ntls_port\n}", 0, true},
		{"dso {\ntls_port 1 2\n}", 0, true},
		{"dso {\ntls_port 65536\n}", 0, true},
		{"dso {\ntls_port -1\n}", -1, true},
		{"dso {\ntls_port 65535\n}", 65535, false},
	} {
		t.Run(test.config, func(t *testing.T) {
			c := caddy.NewTestController("dns", test.config)
			cfg, err := parseConfig(c)
			switch {
			case test.wantErr && err == nil:
				t.Error("Want error")
			case !test.wantErr && err != nil:
				t.Errorf("Got %v", err)
			case err == nil:
				if cfg.TLSPort != test.port {
					t.Errorf("Got TLSPort=%v, want %v", cfg.TLSPort, test.port)
				}
			}
		})
	}
}

func TestConfigParseKeepAlive(t *testing.T) {
	for _, test := range []struct {
		config     string
		keepalive  time.Duration
		inactivity time.Duration
		wantErr    bool
	}{
		{"dso {\ntcp_port 8080\nkeepalive\n}", 0, 0, true},
		{"dso {\ntcp_port 8080\nkeepalive bla\n}", 0, 0, true},
		{"dso {\ntcp_port 8080\nkeepalive 42s bla\n}", 0, 0, true},
		{"dso {\ntcp_port 8080\nkeepalive bla 42s\n}", 0, 0, true},
		{"dso {\ntcp_port 8080\nkeepalive 1 2 3\n}", 0, 0, true},
		{"dso {\ntcp_port 8080\nkeepalive 5s\n}", 0, 0, true},
		{"dso {\ntcp_port 8080\nkeepalive 5s 9000s\n}", 0, 0, true},
		{"dso {\ntcp_port 8080\nkeepalive 42s\n}", 42 * time.Second, 42 * time.Second, false},
		{"dso {\ntcp_port 8080\nkeepalive 42s 9000s\n}", 42 * time.Second, 9000 * time.Second, false},
	} {
		t.Run(test.config, func(t *testing.T) {
			c := caddy.NewTestController("dns", test.config)
			cfg, err := parseConfig(c)
			switch {
			case test.wantErr && err == nil:
				t.Error("Want error")
			case !test.wantErr && err != nil:
				t.Errorf("Got %v", err)
			case err == nil:
				if cfg.KeepAliveInterval != test.keepalive {
					t.Errorf("Got KeepAliveInterval=%v, want %v", cfg.KeepAliveInterval, test.keepalive)
				}
				if cfg.InactivityTimeout != test.inactivity {
					t.Errorf("Got InactivityTimeout=%v, want %v", cfg.InactivityTimeout, test.inactivity)
				}
			}
		})
	}
}

func TestConfigParseReconnect(t *testing.T) {
	for _, test := range []struct {
		config   string
		restart  time.Duration
		shutdown time.Duration
		wantErr  bool
	}{
		{"dso {\ntcp_port 8080\nreconnect\n}", 0, 0, true},
		{"dso {\ntcp_port 8080\nreconnect bla\n}", 0, 0, true},
		{"dso {\ntcp_port 8080\nreconnect 42s bla\n}", 0, 0, true},
		{"dso {\ntcp_port 8080\nreconnect bla 42s\n}", 0, 0, true},
		{"dso {\ntcp_port 8080\nreconnect 42s\n}", 0, 0, true},
		{"dso {\ntcp_port 8080\nreconnect 42s 9000s\n}", 42 * time.Second, 9000 * time.Second, false},
	} {
		t.Run(test.config, func(t *testing.T) {
			c := caddy.NewTestController("dns", test.config)
			cfg, err := parseConfig(c)
			switch {
			case test.wantErr && err == nil:
				t.Error("Want error")
			case !test.wantErr && err != nil:
				t.Errorf("Got %v", err)
			case err == nil:
				if cfg.ShutdownReconnectInterval != test.shutdown {
					t.Errorf("Got ShutdownReconnectInterval=%v, want %v", cfg.ShutdownReconnectInterval, test.shutdown)
				}
				if cfg.RestartReconnectInterval != test.restart {
					t.Errorf("Got RestartReconnectInterval=%v, want %v", cfg.RestartReconnectInterval, test.restart)
				}
			}
		})
	}
}

func TestConfigParsePushUnknown(t *testing.T) {
	c := caddy.NewTestController("dns", "dso {\npush {\nunknown\n}\n}")
	_, err := parseConfig(c)
	if err == nil {
		t.Error("Want error")
	}
}

func TestConfigParsePushZones(t *testing.T) {
	for _, test := range []struct {
		config string
		zones  []string
	}{
		{"dso {\ntls_port 8080\npush {\n}\n\n}", []string{"test."}},
		{"dso {\ntls_port 8080\npush a.test. {\n}\n}", []string{"a.test."}},
	} {
		t.Run(test.config, func(t *testing.T) {
			c := caddy.NewTestController("dns", test.config)
			c.ServerBlockKeys = []string{"dns://test."}
			cfg, err := parseConfig(c)
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			} else if !slices.Equal(cfg.Push.Zones, test.zones) {
				t.Errorf("Zones mismatch:\n%s", cmp.Diff(test.zones, cfg.Push.Zones))
			}
		})
	}
}

func TestConfigParsePushClasses(t *testing.T) {
	for _, test := range []struct {
		config  string
		classes []uint16
		wantErr bool
	}{
		{"dso {\ntls_port 8080\npush {\nclasses\n}\n}", nil, true},
		{"dso {\ntls_port 8080\npush {\nclasses FOO\n}\n}", nil, true},
		{"dso {\ntls_port 8080\npush {\nclasses IN\n}\n}", []uint16{dns.ClassINET}, false},
		{"dso {\ntls_port 8080\npush {\nclasses IN CH\n}\n}", []uint16{dns.ClassINET, dns.ClassCHAOS}, false},
	} {
		t.Run(test.config, func(t *testing.T) {
			c := caddy.NewTestController("dns", test.config)
			cfg, err := parseConfig(c)
			switch {
			case test.wantErr && err == nil:
				t.Error("Want error")
			case !test.wantErr && err != nil:
				t.Errorf("Got %v", err)
			case err == nil:
				if !slices.Equal(cfg.Push.Classes, test.classes) {
					t.Errorf("Classes mismatch:\n%s", cmp.Diff(test.classes, cfg.Push.Classes))
				}
			}
		})
	}
}

func TestConfigParsePushTypes(t *testing.T) {
	for _, test := range []struct {
		config  string
		types   []uint16
		wantErr bool
	}{
		{"dso {\ntls_port 8080\npush {\ntypes\n}\n}", nil, true},
		{"dso {\ntls_port 8080\npush {\ntypes FOO\n}\n}", nil, true},
		{"dso {\ntls_port 8080\npush {\ntypes A\n}\n}", []uint16{dns.TypeA}, false},
		{"dso {\ntls_port 8080\npush {\ntypes A AAAA\n}\n}", []uint16{dns.TypeA, dns.TypeAAAA}, false},
	} {
		t.Run(test.config, func(t *testing.T) {
			c := caddy.NewTestController("dns", test.config)
			cfg, err := parseConfig(c)
			switch {
			case test.wantErr && err == nil:
				t.Error("Want error")
			case !test.wantErr && err != nil:
				t.Errorf("Got %v", err)
			case err == nil:
				if !slices.Equal(cfg.Push.Types, test.types) {
					t.Errorf("Types mismatch:\n%s", cmp.Diff(test.types, cfg.Push.Types))
				}
			}
		})
	}
}

func TestConfigParsePushRefresh(t *testing.T) {
	for _, test := range []struct {
		config  string
		refresh time.Duration
		wantErr bool
	}{
		{"dso {\ntls_port 8080\npush {\nrefresh\n}\n}", 0, true},
		{"dso {\ntls_port 8080\npush {\nrefresh -1\n}\n}", 0, true},
		{"dso {\ntls_port 8080\npush {\nrefresh 42s\n}\n}", 42 * time.Second, false},
	} {
		t.Run(test.config, func(t *testing.T) {
			c := caddy.NewTestController("dns", test.config)
			cfg, err := parseConfig(c)
			switch {
			case test.wantErr && err == nil:
				t.Error("Want error")
			case !test.wantErr && err != nil:
				t.Errorf("Got %v", err)
			case err == nil:
				if cfg.Push.RefreshInterval != test.refresh {
					t.Errorf("Got RefreshInterval=%v, want %v", cfg.Push.RefreshInterval, test.refresh)
				}
			}
		})
	}
}

func TestConfigParsePushDebounce(t *testing.T) {
	for _, test := range []struct {
		config   string
		debounce time.Duration
		wantErr  bool
	}{
		{"dso {\ntls_port 8080\npush {\ndebounce\n}\n}", 0, true},
		{"dso {\ntls_port 8080\npush {\ndebounce -1\n}\n}", 0, true},
		{"dso {\ntls_port 8080\npush {\ndebounce 42s\n}\n}", 42 * time.Second, false},
	} {
		t.Run(test.config, func(t *testing.T) {
			c := caddy.NewTestController("dns", test.config)
			cfg, err := parseConfig(c)
			switch {
			case test.wantErr && err == nil:
				t.Error("Want error")
			case !test.wantErr && err != nil:
				t.Errorf("Got %v", err)
			case err == nil:
				if cfg.Push.DebounceDelay != test.debounce {
					t.Errorf("Got DebounceDelay=%v, want %v", cfg.Push.DebounceDelay, test.debounce)
				}
			}
		})
	}
}

func TestConfigParseLog(t *testing.T) {
	for _, test := range []struct {
		config  string
		wantErr bool
	}{
		{"dso {\ntcp_port 8080\nlog\n}", false},
	} {
		t.Run(test.config, func(t *testing.T) {
			c := caddy.NewTestController("dns", test.config)
			cfg, err := parseConfig(c)
			switch {
			case test.wantErr && err == nil:
				t.Error("Want error")
			case !test.wantErr && err != nil:
				t.Errorf("Got %v", err)
			case err == nil:
				if cfg.Log == nil {
					t.Error("Got Log=<nil>")
				}
			}
		})
	}
}

func TestConfigParseSentinel(t *testing.T) {
	c := caddy.NewTestController("dns", "dso")
	cfg, err := parseConfig(c)
	if err != nil {
		t.Fatalf("Got %v, err", err)
	}
	if cfg != handlerOnlyConfig {
		t.Errorf("Got %v, want sentinel (%v)", cfg, handlerOnlyConfig)
	}
}

func TestConfigParseArgs(t *testing.T) {
	c := caddy.NewTestController("dns", "dso foo {\ntcp_port 8080\n}")
	_, err := parseConfig(c)
	if err == nil {
		t.Error("Want error")
	}
}

func TestConfigOnlyOnce(t *testing.T) {
	c := caddy.NewTestController("dns", "dso {\ntcp_port 8080\n}\ndso {\ntcp_port 8081\n}")
	_, err := parseConfig(c)
	if err == nil {
		t.Error("Want error")
	}
}

func TestConfigRequiresPort(t *testing.T) {
	c := caddy.NewTestController("dns", "dso {\nlog\n}")
	_, err := parseConfig(c)
	if err == nil {
		t.Error("Want error")
	}
}

func TestConfigPushRequiresTLSPort(t *testing.T) {
	c := caddy.NewTestController("dns", "dso {\ntcp_port 8080\npush\n}")
	_, err := parseConfig(c)
	if err == nil {
		t.Error("Want error")
	}
}
