package file

import (
	"net"
	"testing"

	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type rawAddr string

func (a rawAddr) Network() string { return "udp" }
func (a rawAddr) String() string  { return string(a) }

type rawAddrResponseWriter struct {
	test.ResponseWriter
	addr rawAddr
}

func (w *rawAddrResponseWriter) RemoteAddr() net.Addr { return w.addr }

func TestIsNotifyAddressForms(t *testing.T) {
	tests := []struct {
		name         string
		opcode       int
		transferFrom []string
		remoteIP     string
		remoteZone   string
		rawRemote    string // when set, returned verbatim by RemoteAddr().String()
		want         bool
	}{
		{
			name:         "canonical IPv4 match",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"192.0.2.1:53"},
			remoteIP:     "192.0.2.1",
			want:         true,
		},
		{
			name:         "canonical IPv4 non-default port match",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"192.0.2.1:5300"},
			remoteIP:     "192.0.2.1",
			want:         true,
		},
		{
			name:         "canonical IPv4 mismatch",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"192.0.2.2:53"},
			remoteIP:     "192.0.2.1",
			want:         false,
		},
		{
			name:         "canonical IPv6 match",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[2001:db8::1]:53"},
			remoteIP:     "2001:db8::1",
			want:         true,
		},
		{
			name:         "canonical IPv6 mismatch",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[2001:db8::2]:53"},
			remoteIP:     "2001:db8::1",
			want:         false,
		},
		{
			name:         "non-canonical uppercase IPv6 match",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[2001:DB8::1]:53"},
			remoteIP:     "2001:db8::1",
			want:         true,
		},
		{
			name:         "non-canonical uppercase IPv6 non-default port match",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[2001:DB8::1]:5300"},
			remoteIP:     "2001:db8::1",
			want:         true,
		},
		{
			name:         "non-canonical leading zeros IPv6 match",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[2001:0DB8:0000:0000::1]:53"},
			remoteIP:     "2001:db8::1",
			want:         true,
		},
		{
			name:         "uncompressed IPv6 loopback match",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[0:0:0:0:0:0:0:1]:53"},
			remoteIP:     "::1",
			want:         true,
		},
		{
			name:         "uncompressed IPv6 mismatch",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[2001:0db8:0000:0000:0000:0000:0000:0001]:53"},
			remoteIP:     "2001:db8::2",
			want:         false,
		},
		{
			name:         "zoned link-local IPv6 non-canonical match",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[FE80:0:0:0::1%eth0]:53"},
			remoteIP:     "fe80::1",
			remoteZone:   "eth0",
			want:         true,
		},
		{
			name:         "zoned link-local IPv6 different zone mismatch",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[fe80::1%eth1]:53"},
			remoteIP:     "fe80::1",
			remoteZone:   "eth0",
			want:         false,
		},
		{
			name:         "zoned remote without zone in TransferFrom mismatch",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[fe80::1]:53"},
			remoteIP:     "fe80::1",
			remoteZone:   "eth0",
			want:         false,
		},
		{
			name:         "zoned TransferFrom without zone in remote mismatch",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[fe80::1%eth0]:53"},
			remoteIP:     "fe80::1",
			want:         false,
		},
		{
			name:         "IPv4-mapped IPv6 in TransferFrom matches IPv4 remote",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[::ffff:192.0.2.1]:53"},
			remoteIP:     "192.0.2.1",
			want:         true,
		},
		{
			name:         "hex IPv4-mapped IPv6 in TransferFrom matches IPv4 remote",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[::FFFF:c000:0201]:53"},
			remoteIP:     "192.0.2.1",
			want:         true,
		},
		{
			name:         "IPv4 in TransferFrom matches IPv4-mapped IPv6 remote",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"192.0.2.1:53"},
			rawRemote:    "[::ffff:192.0.2.1]:40212",
			want:         true,
		},
		{
			name:         "IPv4-mapped IPv6 in TransferFrom mismatches different IPv4 remote",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[::ffff:192.0.2.2]:53"},
			remoteIP:     "192.0.2.1",
			want:         false,
		},
		{
			name:         "IPv4 in TransferFrom mismatches different IPv4-mapped IPv6 remote",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"192.0.2.2:53"},
			rawRemote:    "[::ffff:192.0.2.1]:40212",
			want:         false,
		},
		{
			name:         "IPv6 loopback does not match IPv4 loopback",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"[::1]:53"},
			remoteIP:     "127.0.0.1",
			want:         false,
		},
		{
			name:         "malformed TransferFrom entries skipped before valid match",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"missing-port", ":53", "not-an-ip:53", "[2001:DB8::1]:53"},
			remoteIP:     "2001:db8::1",
			want:         true,
		},
		{
			name:         "only malformed TransferFrom entries rejected",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"2001:db8::1", "[2001:db8::1]", ":53", "not-an-ip:53"},
			remoteIP:     "2001:db8::1",
			want:         false,
		},
		{
			name:         "invalid remote never matches identical invalid TransferFrom",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"not-an-ip:53"},
			rawRemote:    "not-an-ip:40212",
			want:         false,
		},
		{
			name:         "empty remote never matches empty-host TransferFrom",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{":53"},
			rawRemote:    ":40212",
			want:         false,
		},
		{
			name:         "empty remote address rejected",
			opcode:       dns.OpcodeNotify,
			transferFrom: []string{"192.0.2.1:53"},
			rawRemote:    "",
			want:         false,
		},
		{
			name:         "non-notify opcode rejected even when IP matches",
			opcode:       dns.OpcodeQuery,
			transferFrom: []string{"192.0.2.1:53"},
			remoteIP:     "192.0.2.1",
			want:         false,
		},
		{
			name:         "empty TransferFrom rejected",
			opcode:       dns.OpcodeNotify,
			transferFrom: nil,
			remoteIP:     "192.0.2.1",
			want:         false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			z := &Zone{
				origin:       testZone,
				TransferFrom: tc.transferFrom,
			}

			m := new(dns.Msg)
			m.SetQuestion(testZone, dns.TypeSOA)
			m.Opcode = tc.opcode

			var w dns.ResponseWriter
			if tc.rawRemote != "" || tc.remoteIP == "" {
				w = &rawAddrResponseWriter{addr: rawAddr(tc.rawRemote)}
			} else {
				w = &test.ResponseWriter{RemoteIP: tc.remoteIP, Zone: tc.remoteZone}
			}

			state := request.Request{W: w, Req: m}
			if got := z.isNotify(state); got != tc.want {
				t.Errorf("isNotify(transferFrom=%v, remote=%q) = %v, want %v", tc.transferFrom, state.IP(), got, tc.want)
			}
		})
	}
}
