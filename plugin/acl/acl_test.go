package acl

import (
	"context"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

type testResponseWriter struct {
	test.ResponseWriter
	Rcode int
	Msg   *dns.Msg
}

func (t *testResponseWriter) setRemoteIP(ip string) {
	t.RemoteIP = ip
}

func (t *testResponseWriter) setZone(zone string) {
	t.Zone = zone
}

// WriteMsg implement dns.ResponseWriter interface.
func (t *testResponseWriter) WriteMsg(m *dns.Msg) error {
	t.Rcode = m.Rcode
	t.Msg = m
	return nil
}

func NewTestControllerWithZones(input string, zones []string) *caddy.Controller {
	ctr := caddy.NewTestController("dns", input)
	ctr.ServerBlockKeys = append(ctr.ServerBlockKeys, zones...)
	return ctr
}

func TestACLServeDNS(t *testing.T) {
	type args struct {
		domain   string
		sourceIP string
		qtype    uint16
	}
	tests := []struct {
		name                  string
		config                string
		zones                 []string
		args                  args
		wantRcode             int
		wantErr               bool
		wantExtendedErrorCode uint16
		expectNoResponse      bool
	}{
		// IPv4 tests.
		{
			name: "Blacklist 1 BLOCKED",
			config: `acl example.org {
				block type A net 192.168.0.0/16
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "192.168.0.2",
				qtype:    dns.TypeA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Blacklist 1 ALLOWED",
			config: `acl example.org {
				block type A net 192.168.0.0/16
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "192.167.0.2",
				qtype:    dns.TypeA,
			},
			wantRcode: dns.RcodeSuccess,
		},
		{
			name: "Blacklist 2 BLOCKED",
			config: `
			acl example.org {
				block type * net 192.168.0.0/16
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "192.168.0.2",
				qtype:    dns.TypeAAAA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Blacklist 3 BLOCKED",
			config: `acl example.org {
				block type A
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "10.1.0.2",
				qtype:    dns.TypeA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Blacklist 3 ALLOWED",
			config: `acl example.org {
				block type A
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "10.1.0.2",
				qtype:    dns.TypeAAAA,
			},
			wantRcode: dns.RcodeSuccess,
		},
		{
			name: "Blacklist 4 Single IP BLOCKED",
			config: `acl example.org {
				block type A net 192.168.1.2
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "192.168.1.2",
				qtype:    dns.TypeA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Blacklist 4 Single IP ALLOWED",
			config: `acl example.org {
				block type A net 192.168.1.2
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "192.168.1.3",
				qtype:    dns.TypeA,
			},
			wantRcode: dns.RcodeSuccess,
		},
		{
			name: "Filter 1 FILTERED",
			config: `acl example.org {
				filter type A net 192.168.0.0/16
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "192.168.0.2",
				qtype:    dns.TypeA,
			},
			wantRcode:             dns.RcodeSuccess,
			wantExtendedErrorCode: dns.ExtendedErrorCodeFiltered,
		},
		{
			name: "Filter 1 ALLOWED",
			config: `acl example.org {
				filter type A net 192.168.0.0/16
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "192.167.0.2",
				qtype:    dns.TypeA,
			},
			wantRcode: dns.RcodeSuccess,
		},
		{
			name: "Whitelist 1 ALLOWED",
			config: `acl example.org {
				allow net 192.168.0.0/16
				block
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "192.168.0.2",
				qtype:    dns.TypeA,
			},
			wantRcode: dns.RcodeSuccess,
		},
		{
			name: "Whitelist 1 REFUSED",
			config: `acl example.org {
				allow type * net 192.168.0.0/16
				block
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "10.1.0.2",
				qtype:    dns.TypeA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Drop 1 DROPPED",
			config: `acl example.org {
				drop net 192.168.0.0/16
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "192.168.0.2",
				qtype:    dns.TypeA,
			},
			wantRcode:        dns.RcodeSuccess,
			expectNoResponse: true,
		},
		{
			name: "Subnet-Order 1 REFUSED",
			config: `acl example.org {
				block net 192.168.1.0/24
				drop net 192.168.0.0/16
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "192.168.1.2",
				qtype:    dns.TypeA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Subnet-Order 2 DROPPED",
			config: `acl example.org {
				drop net 192.168.0.0/16
				block net 192.168.1.0/24
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "192.168.1.1",
				qtype:    dns.TypeA,
			},
			wantRcode:        dns.RcodeSuccess,
			expectNoResponse: true,
		},
		{
			name: "Drop-Type 1 DROPPED",
			config: `acl example.org {
				drop type A
				allow net 192.168.0.0/16
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "192.168.1.1",
				qtype:    dns.TypeA,
			},
			wantRcode:        dns.RcodeSuccess,
			expectNoResponse: true,
		},
		{
			name: "Drop-Type 2 ALLOWED",
			config: `acl example.org {
				drop type A
				allow net 192.168.0.0/16
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "192.168.1.1",
				qtype:    dns.TypeAAAA,
			},
			wantRcode: dns.RcodeSuccess,
		},
		{
			name: "Fine-Grained 1 REFUSED",
			config: `acl a.example.org {
				block type * net 192.168.1.0/24
			}`,
			zones: []string{"example.org"},
			args: args{
				domain:   "a.example.org.",
				sourceIP: "192.168.1.2",
				qtype:    dns.TypeA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Fine-Grained 1 ALLOWED",
			config: `acl a.example.org {
				block net 192.168.1.0/24
			}`,
			zones: []string{"example.org"},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "192.168.1.2",
				qtype:    dns.TypeA,
			},
			wantRcode: dns.RcodeSuccess,
		},
		{
			name: "Fine-Grained 2 REFUSED",
			config: `acl example.org {
				block net 192.168.1.0/24
			}`,
			zones: []string{"example.org"},
			args: args{
				domain:   "a.example.org.",
				sourceIP: "192.168.1.2",
				qtype:    dns.TypeA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Fine-Grained 2 ALLOWED",
			config: `acl {
				block net 192.168.1.0/24
			}`,
			zones: []string{"example.org"},
			args: args{
				domain:   "a.example.com.",
				sourceIP: "192.168.1.2",
				qtype:    dns.TypeA,
			},
			wantRcode: dns.RcodeSuccess,
		},
		{
			name: "Fine-Grained 3 REFUSED",
			config: `acl a.example.org {
				block net 192.168.1.0/24
			}
			acl b.example.org {
				block type * net 192.168.2.0/24
			}`,
			zones: []string{"example.org"},
			args: args{
				domain:   "b.example.org.",
				sourceIP: "192.168.2.2",
				qtype:    dns.TypeA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Fine-Grained 3 ALLOWED",
			config: `acl a.example.org {
				block net 192.168.1.0/24
			}
			acl b.example.org {
				block net 192.168.2.0/24
			}`,
			zones: []string{"example.org"},
			args: args{
				domain:   "b.example.org.",
				sourceIP: "192.168.1.2",
				qtype:    dns.TypeA,
			},
			wantRcode: dns.RcodeSuccess,
		},
		// IPv6 tests.
		{
			name: "Blacklist 1 BLOCKED IPv6",
			config: `acl example.org {
				block type A net 2001:db8:abcd:0012::0/64
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "2001:db8:abcd:0012::1230",
				qtype:    dns.TypeA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Blacklist 1 ALLOWED IPv6",
			config: `acl example.org {
				block type A net 2001:db8:abcd:0012::0/64
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "2001:db8:abcd:0013::0",
				qtype:    dns.TypeA,
			},
			wantRcode: dns.RcodeSuccess,
		},
		{
			name: "Blacklist 2 BLOCKED IPv6",
			config: `acl example.org {
				block type A
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
				qtype:    dns.TypeA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Blacklist 3 Single IP BLOCKED IPv6",
			config: `acl example.org {
				block type A net 2001:0db8:85a3:0000:0000:8a2e:0370:7334
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
				qtype:    dns.TypeA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Blacklist 3 Single IP ALLOWED IPv6",
			config: `acl example.org {
				block type A net 2001:0db8:85a3:0000:0000:8a2e:0370:7334
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "2001:0db8:85a3:0000:0000:8a2e:0370:7335",
				qtype:    dns.TypeA,
			},
			wantRcode: dns.RcodeSuccess,
		},
		{
			name: "Fine-Grained 1 REFUSED IPv6",
			config: `acl a.example.org {
				block type * net 2001:db8:abcd:0012::0/64
			}`,
			zones: []string{"example.org"},
			args: args{
				domain:   "a.example.org.",
				sourceIP: "2001:db8:abcd:0012:2019::0",
				qtype:    dns.TypeA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Fine-Grained 1 ALLOWED IPv6",
			config: `acl a.example.org {
				block net 2001:db8:abcd:0012::0/64
			}`,
			zones: []string{"example.org"},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "2001:db8:abcd:0012:2019::0",
				qtype:    dns.TypeA,
			},
			wantRcode: dns.RcodeSuccess,
		},
		{
			name: "Blacklist Address%ifname",
			config: `acl example.org {
				block type AAAA net 2001:0db8:85a3:0000:0000:8a2e:0370:7334
			}`,
			zones: []string{"eth0"},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
				qtype:    dns.TypeAAAA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Drop 1 DROPPED IPV6",
			config: `acl example.org {
				drop net 2001:0db8:85a3:0000:0000:8a2e:0370:7334
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
				qtype:    dns.TypeAAAA,
			},
			wantRcode:        dns.RcodeSuccess,
			expectNoResponse: true,
		},
		{
			name: "Subnet-Order 1 REFUSED IPv6",
			config: `acl example.org {
				block net 2001:db8:abcd:0012:8000::/66
				drop net 2001:db8:abcd:0012::0/64
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "2001:db8:abcd:0012:8000::1",
				qtype:    dns.TypeAAAA,
			},
			wantRcode:             dns.RcodeRefused,
			wantExtendedErrorCode: dns.ExtendedErrorCodeBlocked,
		},
		{
			name: "Subnet-Order 2 DROPPED IPv6",
			config: `acl example.org {
				drop net 2001:db8:abcd:0012::0/64
				block net 2001:db8:abcd:0012:8000::/66
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "2001:db8:abcd:0012:8000::1",
				qtype:    dns.TypeAAAA,
			},
			wantRcode:        dns.RcodeSuccess,
			expectNoResponse: true,
		},
		{
			name: "Drop-Type 1 DROPPED IPv6",
			config: `acl example.org {
				drop type A
				allow net 2001:db8:85a3:0000::0/64
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
				qtype:    dns.TypeA,
			},
			wantRcode:        dns.RcodeSuccess,
			expectNoResponse: true,
		},
		{
			name: "Drop-Type 2 ALLOWED IPv6",
			config: `acl example.org {
				drop type A
				allow net 2001:db8:85a3:0000::0/64
			}`,
			zones: []string{},
			args: args{
				domain:   "www.example.org.",
				sourceIP: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
				qtype:    dns.TypeAAAA,
			},
			wantRcode: dns.RcodeSuccess,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctr := NewTestControllerWithZones(tt.config, tt.zones)
			a, err := parse(ctr)
			a.Next = test.NextHandler(dns.RcodeSuccess, nil)
			if err != nil {
				t.Errorf("Error: Cannot parse acl from config: %v", err)
				return
			}

			w := &testResponseWriter{}
			m := new(dns.Msg)
			w.setRemoteIP(tt.args.sourceIP)
			if len(tt.zones) > 0 {
				w.setZone(tt.zones[0])
			}
			m.SetQuestion(tt.args.domain, tt.args.qtype)
			_, err = a.ServeDNS(ctx, w, m)
			if (err != nil) != tt.wantErr {
				t.Errorf("Error: acl.ServeDNS() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if w.Rcode != tt.wantRcode {
				t.Errorf("Error: acl.ServeDNS() Rcode = %v, want %v", w.Rcode, tt.wantRcode)
			}
			if tt.expectNoResponse && w.Msg != nil {
				t.Errorf("Error: acl.ServeDNS() responded to client when not expected")
			}
			if tt.wantExtendedErrorCode != 0 {
				matched := false
				for _, opt := range w.Msg.IsEdns0().Option {
					if ede, ok := opt.(*dns.EDNS0_EDE); ok {
						if ede.InfoCode != tt.wantExtendedErrorCode {
							t.Errorf("Error: acl.ServeDNS() Extended DNS Error = %v, want %v", ede.InfoCode, tt.wantExtendedErrorCode)
						}
						matched = true
					}
				}
				if !matched {
					t.Error("Error: acl.ServeDNS() missing Extended DNS Error option")
				}
			}
		})
	}
}
