package acl

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/caddyserver/caddy"
	"github.com/miekg/dns"
)

var aclTestFiles = map[string]string{
	"acl-test-1.txt": `192.168.1.0/24`,
}

type testResponseWriter struct {
	test.ResponseWriter
	Rcode int
}

func (t *testResponseWriter) setRemoteIP(ip string) {
	t.RemoteIP = ip
}

// WriteMsg implement dns.ResponseWriter interface.
func (t *testResponseWriter) WriteMsg(m *dns.Msg) error {
	t.Rcode = m.Rcode
	return nil
}

func NewTestControllerWithZones(input string, zones []string) *caddy.Controller {
	ctr := caddy.NewTestController("dns", input)
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
		config    string
		zones     []string
		args      args
		wantRcode int
		wantErr   bool
	}{
		{
			"Blacklist 1 BLOCKED",
			`acl example.org {
				block type A net 192.168.0.0/16
			}`,
			[]string{},
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
			`acl example.org {
				block type A net 192.168.0.0/16
			}`,
			[]string{},
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
			`
			acl example.org {
				block type * net 192.168.0.0/16
			}`,
			[]string{},
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
			`acl example.org {
				block type A
			}`,
			[]string{},
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
			`acl example.org {
				block type A
			}`,
			[]string{},
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
			`acl example.org {
				block type A net 192.168.1.2
			}`,
			[]string{},
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
			`acl example.org {
				block type A net 192.168.1.2
			}`,
			[]string{},
			args{
				"www.example.org.",
				"192.168.1.3",
				dns.TypeA,
			},
			dns.RcodeSuccess,
			false,
		},
		{
			"Whitelist 1 ALLOWED",
			`acl example.org {
				allow net 192.168.0.0/16
				block
			}`,
			[]string{},
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
			`acl example.org {
				allow type * net 192.168.0.0/16
				block
			}`,
			[]string{},
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
			`acl a.example.org {
				block type * net 192.168.1.0/24
			}`,
			[]string{"example.org"},
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
			`acl a.example.org {
				block net 192.168.1.0/24
			}`,
			[]string{"example.org"},
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
			`acl {
				block net 192.168.1.0/24
			}`,
			[]string{"example.org"},
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
			`acl {
				block net 192.168.1.0/24
			}`,
			[]string{"example.org"},
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
			`acl a.example.org {
				block net 192.168.1.0/24
			}
			acl b.example.org {
				block type * net 192.168.2.0/24
			}`,
			[]string{"example.org"},
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
			`acl a.example.org {
				block net 192.168.1.0/24
			}
			acl b.example.org {
				block net 192.168.2.0/24
			}`,
			[]string{"example.org"},
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
			`acl example.com {
				block file acl-test-1.txt
			}`,
			[]string{},
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
			`acl example.com {
				block file acl-test-1.txt
			}`,
			[]string{},
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
			ctr := NewTestControllerWithZones(tt.config, tt.zones)
			a, err := parse(ctr)
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
