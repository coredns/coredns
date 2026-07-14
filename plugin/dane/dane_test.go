package dane

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

const dbMiekNL = `
$TTL    30M
$ORIGIN miek.nl.
@       IN      SOA     linode.atoom.net. miek.miek.nl. (
                             1282630057 ; Serial
                             4H         ; Refresh
                             1H         ; Retry
                             7D         ; Expire
                             4H )       ; Negative Cache TTL
                        A 1.1.1.1
_25._tcp                TLSA    3 1 1 01
_26._tcp                TLSA    2 0 1 01
_27._tcp                TLSA    2 0 1 96BCEC06264976F37460779ACF28C5A7CFE8A3C0AAE11A8FFCEE05C0BDDF08C6
_27._tcp                TXT     "asdf"
`

var dnsTestCases = []test.Case{
	{
		Qname: "_25._tcp.miek.nl.", Qtype: dns.TypeTLSA,
		Answer: []dns.RR{
			test.TLSA("_25._tcp.miek.nl.	1800	IN	TLSA	3 1 1 F4"),
		},
	},
	{
		Qname: "_26._tcp.miek.nl.", Qtype: dns.TypeTLSA,
		Answer: []dns.RR{
			test.TLSA("_26._tcp.miek.nl.	1800	IN	TLSA	2 0 1 F1"),
		},
	},
	{
		Qname: "_27._tcp.miek.nl.", Qtype: dns.TypeTLSA,
		Answer: []dns.RR{
			test.TLSA("_27._tcp.miek.nl.	1800	IN	TLSA	2 0 1 96BCEC06264976F37460779ACF28C5A7CFE8A3C0AAE11A8FFCEE05C0BDDF08C6"),
		},
	},
	{
		Qname: "_27._tcp.miek.nl.", Qtype: dns.TypeTXT,
		Answer: []dns.RR{
			test.TXT("_27._tcp.miek.nl.	1800	IN	TXT     \"asdf\""),
		},
	},
	{
		Qname: "miek.nl.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("miek.nl.	1800	IN	A	1.1.1.1"),
		},
	},
}

func TestDane_Replacement(t *testing.T) {
	zone, err := file.Parse(strings.NewReader(dbMiekNL), "miek.nl.", "stdin", 0)
	if err != nil {
		return
	}
	fm := file.File{Next: test.ErrorHandler(), Zones: file.Zones{Z: map[string]*file.Zone{"miek.nl.": zone}, Names: []string{"miek.nl."}}}

	d := Dane{
		Next:  fm,
		Zones: []string{"miek.nl."},
		Certificates: map[string][]string{
			"01": {
				"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8",
			},
		},
	}

	for _, tc := range dnsTestCases {
		m := tc.Msg()

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := d.ServeDNS(context.TODO(), rec, m)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
			return
		}

		if err := test.SortAndCheck(rec.Msg, tc); err != nil {
			t.Error(err)
		}
	}
}
