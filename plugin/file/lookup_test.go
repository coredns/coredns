package file

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var dnsTestCases = []test.Case{
	{
		Qname: "www.miek.nl.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("a.miek.nl.	1800	IN	A	139.162.196.78"),
			test.CNAME("www.miek.nl.	1800	IN	CNAME	a.miek.nl."),
		},
		Ns: miekAuth,
	},
	{
		Qname: "www.miek.nl.", Qtype: dns.TypeAAAA,
		Answer: []dns.RR{
			test.AAAA("a.miek.nl.	1800	IN	AAAA	2a01:7e00::f03c:91ff:fef1:6735"),
			test.CNAME("www.miek.nl.	1800	IN	CNAME	a.miek.nl."),
		},
		Ns: miekAuth,
	},
	{
		Qname: "miek.nl.", Qtype: dns.TypeSOA,
		Answer: []dns.RR{
			test.SOA("miek.nl.	1800	IN	SOA	linode.atoom.net. miek.miek.nl. 1282630057 14400 3600 604800 14400"),
		},
		Ns: miekAuth,
	},
	{
		Qname: "miek.nl.", Qtype: dns.TypeAAAA,
		Answer: []dns.RR{
			test.AAAA("miek.nl.	1800	IN	AAAA	2a01:7e00::f03c:91ff:fef1:6735"),
		},
		Ns: miekAuth,
	},
	{
		Qname: "mIeK.NL.", Qtype: dns.TypeAAAA,
		Answer: []dns.RR{
			test.AAAA("miek.nl.	1800	IN	AAAA	2a01:7e00::f03c:91ff:fef1:6735"),
		},
		Ns: miekAuth,
	},
	{
		Qname: "miek.nl.", Qtype: dns.TypeMX,
		Answer: []dns.RR{
			test.MX("miek.nl.	1800	IN	MX	1 aspmx.l.google.com."),
			test.MX("miek.nl.	1800	IN	MX	10 aspmx2.googlemail.com."),
			test.MX("miek.nl.	1800	IN	MX	10 aspmx3.googlemail.com."),
			test.MX("miek.nl.	1800	IN	MX	5 alt1.aspmx.l.google.com."),
			test.MX("miek.nl.	1800	IN	MX	5 alt2.aspmx.l.google.com."),
		},
		Ns: miekAuth,
	},
	{
		Qname: "a.miek.nl.", Qtype: dns.TypeSRV,
		Ns: []dns.RR{
			test.SOA("miek.nl.	1800	IN	SOA	linode.atoom.net. miek.miek.nl. 1282630057 14400 3600 604800 14400"),
		},
	},
	{
		Qname: "b.miek.nl.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("miek.nl.	1800	IN	SOA	linode.atoom.net. miek.miek.nl. 1282630057 14400 3600 604800 14400"),
		},
	},
	{
		Qname: "srv.miek.nl.", Qtype: dns.TypeSRV,
		Answer: []dns.RR{
			test.SRV("srv.miek.nl.	1800	IN	SRV	10 10 8080  a.miek.nl."),
		},
		Extra: []dns.RR{
			test.A("a.miek.nl.	1800	IN	A       139.162.196.78"),
			test.AAAA("a.miek.nl.	1800	IN	AAAA	2a01:7e00::f03c:91ff:fef1:6735"),
		},
		Ns: miekAuth,
	},
	{
		Qname: "mx.miek.nl.", Qtype: dns.TypeMX,
		Answer: []dns.RR{
			test.MX("mx.miek.nl.	1800	IN	MX	10 a.miek.nl."),
		},
		Extra: []dns.RR{
			test.A("a.miek.nl.	1800	IN	A       139.162.196.78"),
			test.AAAA("a.miek.nl.	1800	IN	AAAA	2a01:7e00::f03c:91ff:fef1:6735"),
		},
		Ns: miekAuth,
	},
	{
		Qname: "asterisk.x.miek.nl.", Qtype: dns.TypeCNAME,
		Answer: []dns.RR{
			test.CNAME("asterisk.x.miek.nl. 1800    IN      CNAME   www.miek.nl."),
		},
		Ns: miekAuth,
	},
	{
		Qname: "a.b.x.miek.nl.", Qtype: dns.TypeCNAME,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("miek.nl.	1800	IN	SOA	linode.atoom.net. miek.miek.nl. 1282630057 14400 3600 604800 14400"),
		},
	},
	{
		Qname: "asterisk.y.miek.nl.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("asterisk.y.miek.nl.     1800    IN      A       139.162.196.78"),
		},
		Ns: miekAuth,
	},
	{
		Qname: "foo.dname.miek.nl.", Qtype: dns.TypeCNAME,
		Answer: []dns.RR{
			test.DNAME("dname.miek.nl.     1800    IN      DNAME       x.miek.nl."),
			test.CNAME("foo.dname.miek.nl.     1800    IN      CNAME       foo.x.miek.nl."),
		},
		Ns: miekAuth,
	},
	{
		Qname: "ext-cname.miek.nl.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.CNAME("ext-cname.miek.nl.	1800	IN	CNAME	example.com."),
		},
		Rcode: dns.RcodeServerFailure,
		Ns:    miekAuth,
	},
	{
		Qname: "txt.miek.nl.", Qtype: dns.TypeTXT,
		Answer: []dns.RR{
			test.TXT(`txt.miek.nl.  1800	IN	TXT "v=spf1 a mx ~all"`),
		},
		Ns: miekAuth,
	},
	{
		Qname: "caa.miek.nl.", Qtype: dns.TypeCAA,
		Answer: []dns.RR{
			test.CAA(`caa.miek.nl.  1800	IN	CAA  0 issue letsencrypt.org`),
		},
		Ns: miekAuth,
	},
}

const (
	testzone  = "miek.nl."
	testzone1 = "dnssex.nl."
)

func TestLookup(t *testing.T) {
	zone, err := Parse(strings.NewReader(dbMiekNL), testzone, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{testzone: zone}, Names: []string{testzone}}}
	ctx := context.TODO()

	for _, tc := range dnsTestCases {
		m := tc.Msg()

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := fm.ServeDNS(ctx, rec, m)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
			return
		}

		resp := rec.Msg
		if err := test.SortAndCheck(resp, tc); err != nil {
			t.Error(err)
		}
	}
}

func TestLookupNil(t *testing.T) {
	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{testzone: nil}, Names: []string{testzone}}}
	ctx := context.TODO()

	m := dnsTestCases[0].Msg()
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	fm.ServeDNS(ctx, rec, m)
}

func TestLookUpNoDataResult(t *testing.T) {
	zone, err := Parse(strings.NewReader(dbMiekNL), testzone, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{testzone: zone}, Names: []string{testzone}}}
	ctx := context.TODO()
	var noDataTestCases = []test.Case{
		{
			Qname: "a.miek.nl.", Qtype: dns.TypeMX,
		},
		{
			Qname: "wildcard.nodata.miek.nl.", Qtype: dns.TypeMX,
		},
	}

	for _, tc := range noDataTestCases {
		m := tc.Msg()
		state := request.Request{W: &test.ResponseWriter{}, Req: m}
		_, _, _, result := fm.Z[testzone].Lookup(ctx, state, tc.Qname)
		if result != NoData {
			t.Errorf("Expected result == 3 but result == %v ", result)
		}
	}
}

func BenchmarkFileLookup(b *testing.B) {
	zone, err := Parse(strings.NewReader(dbMiekNL), testzone, "stdin", 0)
	if err != nil {
		return
	}

	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{testzone: zone}, Names: []string{testzone}}}
	ctx := context.TODO()
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	tc := test.Case{
		Qname: "www.miek.nl.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.CNAME("www.miek.nl.	1800	IN	CNAME	a.miek.nl."),
			test.A("a.miek.nl.	1800	IN	A	139.162.196.78"),
		},
	}

	m := tc.Msg()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fm.ServeDNS(ctx, rec, m)
	}
}

const dbMiekNL = `
$TTL    30M
$ORIGIN miek.nl.
@       IN      SOA     linode.atoom.net. miek.miek.nl. (
                             1282630057 ; Serial
                             4H         ; Refresh
                             1H         ; Retry
                             7D         ; Expire
                             4H )       ; Negative Cache TTL
                IN      NS      linode.atoom.net.
                IN      NS      ns-ext.nlnetlabs.nl.
                IN      NS      omval.tednet.nl.
                IN      NS      ext.ns.whyscream.net.

                IN      MX      1  aspmx.l.google.com.
                IN      MX      5  alt1.aspmx.l.google.com.
                IN      MX      5  alt2.aspmx.l.google.com.
                IN      MX      10 aspmx2.googlemail.com.
                IN      MX      10 aspmx3.googlemail.com.

		IN      A       139.162.196.78
		IN      AAAA    2a01:7e00::f03c:91ff:fef1:6735

a               IN      A       139.162.196.78
                IN      AAAA    2a01:7e00::f03c:91ff:fef1:6735
www             IN      CNAME   a
archive         IN      CNAME   a
*.x             IN      CNAME   www
b.x             IN      CNAME   a
*.y             IN      A       139.162.196.78
dname           IN      DNAME   x

srv		IN	SRV     10 10 8080 a.miek.nl.
mx		IN	MX      10 a.miek.nl.

txt     IN	TXT     "v=spf1 a mx ~all"
caa     IN  CAA    0 issue letsencrypt.org
*.nodata    IN   A      139.162.196.79
ext-cname   IN   CNAME  example.com.`
