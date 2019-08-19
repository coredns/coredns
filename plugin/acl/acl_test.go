package acl

import (
	"context"
	"net"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/caddyserver/caddy"
	"github.com/miekg/dns"
)

var aclTestFiles = map[string]string{
	"acl-test-1.txt": `192.168.1.0/24`,
}

type testResponseWriter struct {
	remoteIP net.Addr
	Rcode    int
}

func (t *testResponseWriter) setRemoteIP(rawIP string) {
	ip := net.ParseIP(rawIP)
	port := 65667
	t.remoteIP = &net.UDPAddr{IP: ip, Port: port, Zone: ""}
}

// LocalAddr returns the local address, 127.0.0.1:53.
func (t *testResponseWriter) LocalAddr() net.Addr {
	ip := net.ParseIP("127.0.0.1")
	port := 53
	return &net.UDPAddr{IP: ip, Port: port, Zone: ""}
}

// RemoteAddr returns the remote address, always 10.240.0.1:40212.
func (t *testResponseWriter) RemoteAddr() net.Addr {
	return t.remoteIP
}

// WriteMsg implement dns.ResponseWriter interface.
func (t *testResponseWriter) WriteMsg(m *dns.Msg) error {
	t.Rcode = m.Rcode
	return nil
}

// Write implement dns.ResponseWriter interface.
func (t *testResponseWriter) Write(buf []byte) (int, error) { return len(buf), nil }

// Close implement dns.ResponseWriter interface.
func (t *testResponseWriter) Close() error { return nil }

// TsigStatus implement dns.ResponseWriter interface.
func (t *testResponseWriter) TsigStatus() error { return nil }

// TsigTimersOnly implement dns.ResponseWriter interface.
func (t *testResponseWriter) TsigTimersOnly(bool) { return }

// Hijack implement dns.ResponseWriter interface.
func (t *testResponseWriter) Hijack() { return }

func NewTestControllerWithZones(serverType, input string, zones []string) *caddy.Controller {
	ctr := caddy.NewTestController(serverType, input)
	for _, zone := range zones {
		ctr.ServerBlockKeys = append(ctr.ServerBlockKeys, zone)
	}
	return ctr
}

func TestACLServeDNS(t *testing.T) {
	envSetup(aclTestFiles)
	defer envCleanup(aclTestFiles)

	type args struct {
		domain   string
		sourceIP string
		qtype    uint16
	}
	tests := []struct {
		name      string
		ctr       *caddy.Controller
		args      args
		wantRcode int
		wantErr   bool
	}{
		{
			"Blacklist 1 BLOCKED",
			caddy.NewTestController("dns", `
			acl example.org {
				block type A net 192.168.0.0/16
			}`),
			args{
				"www.example.org.",
				"192.168.0.2",
				dns.TypeA,
			},
			dns.RcodeRefused,
			false,
		},
		{
			"Blacklist 1 ALLOWED",
			caddy.NewTestController("dns", `
			acl example.org {
				block type A net 192.168.0.0/16
			}`),
			args{
				"www.example.org.",
				"192.167.0.2",
				dns.TypeA,
			},
			dns.RcodeSuccess,
			false,
		},
		{
			"Blacklist 2 BLOCKED",
			caddy.NewTestController("dns", `
			acl example.org {
				block type * net 192.168.0.0/16
			}`),
			args{
				"www.example.org.",
				"192.168.0.2",
				dns.TypeAAAA,
			},
			dns.RcodeRefused,
			false,
		},
		{
			"Blacklist 3 BLOCKED",
			caddy.NewTestController("dns", `
			acl example.org {
				block type A
			}`),
			args{
				"www.example.org.",
				"10.1.0.2",
				dns.TypeA,
			},
			dns.RcodeRefused,
			false,
		},
		{
			"Blacklist 3 ALLOWED",
			caddy.NewTestController("dns", `
			acl example.org {
				block type A
			}`),
			args{
				"www.example.org.",
				"10.1.0.2",
				dns.TypeAAAA,
			},
			dns.RcodeSuccess,
			false,
		},
		{
			"Blacklist 4 Single IP BLOCKED",
			caddy.NewTestController("dns", `
			acl example.org {
				block type A net 192.168.1.2
			}`),
			args{
				"www.example.org.",
				"192.168.1.2",
				dns.TypeA,
			},
			dns.RcodeRefused,
			false,
		},
		{
			"Blacklist 4 Single IP ALLOWED",
			caddy.NewTestController("dns", `
			acl example.org {
				block type A net 192.168.1.2
			}`),
			args{
				"www.example.org.",
				"192.168.1.3",
				dns.TypeAAAA,
			},
			dns.RcodeSuccess,
			false,
		},
		{
			"Whitelist 1 ALLOWED",
			caddy.NewTestController("dns", `
			acl example.org {
				allow type * net 192.168.0.0/16
				block type * net *
			}`),
			args{
				"www.example.org.",
				"192.168.0.2",
				dns.TypeA,
			},
			dns.RcodeSuccess,
			false,
		},
		{
			"Whitelist 1 REFUSED",
			caddy.NewTestController("dns", `
			acl example.org {
				allow type * net 192.168.0.0/16
				block type * net *
			}`),
			args{
				"www.example.org.",
				"10.1.0.2",
				dns.TypeA,
			},
			dns.RcodeRefused,
			false,
		},
		{
			"Fine-Grained 1 REFUSED",
			NewTestControllerWithZones("dns", `
			acl a.example.org {
				block type * net 192.168.1.0/24
			}`, []string{"example.org"}),
			args{
				"a.example.org.",
				"192.168.1.2",
				dns.TypeA,
			},
			dns.RcodeRefused,
			false,
		},
		{
			"Fine-Grained 1 ALLOWED",
			NewTestControllerWithZones("dns", `
			acl a.example.org {
				block type * net 192.168.1.0/24
			}`, []string{"example.org"}),
			args{
				"www.example.org.",
				"192.168.1.2",
				dns.TypeA,
			},
			dns.RcodeSuccess,
			false,
		},
		{
			"Fine-Grained 2 REFUSED",
			NewTestControllerWithZones("dns", `
			acl {
				block type * net 192.168.1.0/24
			}`, []string{"example.org"}),
			args{
				"a.example.org.",
				"192.168.1.2",
				dns.TypeA,
			},
			dns.RcodeRefused,
			false,
		},
		{
			"Fine-Grained 2 ALLOWED",
			NewTestControllerWithZones("dns", `
			acl {
				block type * net 192.168.1.0/24
			}`, []string{"example.org"}),
			args{
				"a.example.com.",
				"192.168.1.2",
				dns.TypeA,
			},
			dns.RcodeSuccess,
			false,
		},
		{
			"Fine-Grained 2 REFUSED",
			NewTestControllerWithZones("dns", `
			acl a.example.org {
				block type * net 192.168.1.0/24
			}
			acl b.example.org {
				block type * net 192.168.2.0/24
			}`, []string{"example.org"}),
			args{
				"b.example.org.",
				"192.168.2.2",
				dns.TypeA,
			},
			dns.RcodeRefused,
			false,
		},
		{
			"Fine-Grained 2 ALLOWED",
			NewTestControllerWithZones("dns", `
			acl a.example.org {
				block type * net 192.168.1.0/24
			}
			acl b.example.org {
				block type * net 192.168.2.0/24
			}`, []string{"example.org"}),
			args{
				"b.example.org.",
				"192.168.1.2",
				dns.TypeA,
			},
			dns.RcodeSuccess,
			false,
		},
		{
			"Local file 1 Blocked",
			caddy.NewTestController("dns", `
			acl example.com {
				block type * file acl-test-1.txt
			}`),
			args{
				"a.example.com.",
				"192.168.1.2",
				dns.TypeA,
			},
			dns.RcodeRefused,
			false,
		},
		{
			"Local file 1 Allowed",
			caddy.NewTestController("dns", `
			acl example.com {
				block type * file acl-test-1.txt
			}`),
			args{
				"a.example.com.",
				"192.168.3.1",
				dns.TypeA,
			},
			dns.RcodeSuccess,
			false,
		},
		// TODO: Add more test cases. (@ihac)
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := parse(tt.ctr)
			a.Next = test.NextHandler(dns.RcodeSuccess, nil)
			if err != nil {
				t.Errorf("Error: Cannot parse acl from config: %v", err)
			}

			w := &testResponseWriter{}
			m := new(dns.Msg)
			w.setRemoteIP(tt.args.sourceIP)
			m.SetQuestion(tt.args.domain, tt.args.qtype)
			_, err = a.ServeDNS(ctx, w, m)
			if (err != nil) != tt.wantErr {
				t.Errorf("Error: acl.ServeDNS() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if w.Rcode != tt.wantRcode {
				t.Errorf("Error: acl.ServeDNS() Rcode = %v, want %v", w.Rcode, tt.wantRcode)
			}
		})
	}
}
