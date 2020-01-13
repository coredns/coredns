package hosts

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestLookupA(t *testing.T) {
	for _, tc := range hostsTestCases {
		m := tc.Msg()

		var tcFall fall.F
		isFall := tc.Qname == "fallthrough-example.org."
		if isFall {
			tcFall = fall.Root
		} else {
			tcFall = fall.Zero
		}

		h := Hosts{
			Next: test.NextHandler(dns.RcodeNameError, nil),
			Hostsfile: &Hostsfile{
				Origins: []string{"."},
				hmap:    newMap(),
				inline:  newMap(),
				options: newOptions(),
			},
			Fall: tcFall,
		}
		h.hmap = h.parse(strings.NewReader(hostsExample))

		rec := dnstest.NewRecorder(&test.ResponseWriter{})

		rcode, err := h.ServeDNS(context.Background(), rec, m)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
			return
		}

		if isFall && tc.Rcode != rcode {
			t.Errorf("Expected rcode is %d, but got %d", tc.Rcode, rcode)
			return
		}

		if resp := rec.Msg; rec.Msg != nil {
			if err := test.SortAndCheck(resp, tc); err != nil {
				t.Error(err)
			}
		}
	}
}

var hostsTestCases = []test.Case{
	{
		Qname: "example.org.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("example.org. 3600	IN	A 10.0.0.1"),
		},
	},
	{
		Qname: "example.com.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("example.com. 3600	IN	A 10.0.0.2"),
		},
	},
	{
		Qname: "localhost.", Qtype: dns.TypeAAAA,
		Answer: []dns.RR{
			test.AAAA("localhost. 3600	IN	AAAA ::1"),
		},
	},
	{
		Qname: "1.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
		Answer: []dns.RR{
			test.PTR("1.0.0.10.in-addr.arpa. 3600 PTR example.org."),
		},
	},
	{
		Qname: "2.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
		Answer: []dns.RR{
			test.PTR("2.0.0.10.in-addr.arpa. 3600 PTR example.com."),
		},
	},
	{
		Qname: "1.0.0.127.in-addr.arpa.", Qtype: dns.TypePTR,
		Answer: []dns.RR{
			test.PTR("1.0.0.127.in-addr.arpa. 3600 PTR localhost."),
			test.PTR("1.0.0.127.in-addr.arpa. 3600 PTR localhost.domain."),
		},
	},
	{
		Qname: "example.org.", Qtype: dns.TypeAAAA,
		Answer: []dns.RR{},
	},
	{
		Qname: "example.org.", Qtype: dns.TypeMX,
		Answer: []dns.RR{},
	},
	{
		Qname: "fallthrough-example.org.", Qtype: dns.TypeAAAA,
		Answer: []dns.RR{}, Rcode: dns.RcodeSuccess,
	},
}

const hostsExample = `
127.0.0.1 localhost localhost.domain
::1 localhost localhost.domain
10.0.0.1 example.org
::FFFF:10.0.0.2 example.com
10.0.0.3 fallthrough-example.org
reload 5s
timeout 3600
`
