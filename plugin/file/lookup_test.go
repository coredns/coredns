package file

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/fall"
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

func TestLookupNil(_t *testing.T) {
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

func TestLookupFallthrough(t *testing.T) {
	zone, err := Parse(strings.NewReader(dbMiekNL), testzone, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	type FallWithTestCases struct {
		Fall  fall.F
		Cases []test.Case
	}
	var fallsWithTestCases = []FallWithTestCases{
		{
			Fall: fall.Root,
			Cases: []test.Case{
				{
					Qname: "doesnotexist.miek.nl.", Qtype: dns.TypeA,
					Rcode: dns.RcodeServerFailure,
				},
				{
					Qname: "x.miek.nl.", Qtype: dns.TypeA,
					Rcode: dns.RcodeServerFailure,
				},
			},
		},
		{
			Fall: fall.F{Zones: []string{"a.miek.nl."}},
			Cases: []test.Case{
				{
					Qname: "a.miek.nl.", Qtype: dns.TypeA,
					Rcode: dns.RcodeSuccess,
				},
				{
					Qname: "doesnotexist.miek.nl.", Qtype: dns.TypeA,
					Rcode: dns.RcodeNameError,
				},
				{
					Qname: "passthrough.a.miek.nl.", Qtype: dns.TypeA,
					Rcode:  dns.RcodeServerFailure,
					Answer: []dns.RR{},
				},
			},
		},
		{
			Fall: fall.F{Zones: []string{"x.miek.nl."}},
			Cases: []test.Case{
				{
					Qname: "x.miek.nl.", Qtype: dns.TypeA,
					Rcode: dns.RcodeServerFailure,
				},
				{
					Qname: "wildcard.x.miek.nl.", Qtype: dns.TypeA,
					Rcode: dns.RcodeSuccess,
				},
			},
		},
	}

	for _, fallWithTestCases := range fallsWithTestCases {
		fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{testzone: zone}, Names: []string{testzone}}, Fall: fallWithTestCases.Fall}
		ctx := context.TODO()

		for _, tc := range fallWithTestCases.Cases {
			m := tc.Msg()

			rec := dnstest.NewRecorder(&test.ResponseWriter{})
			_, err := fm.ServeDNS(ctx, rec, m)
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
				return
			}

			if rec.Msg.Rcode != tc.Rcode {
				t.Errorf("rcode is %q, expected %q", dns.RcodeToString[rec.Msg.Rcode], dns.RcodeToString[tc.Rcode])
				return
			}
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

	for b.Loop() {
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

var additionalAuth = []dns.RR{
	test.NS("example.org. 1800 IN NS ns.example.org."),
}

var additionalTestCases = []test.Case{
	{
		// Two MX records that only differ in preference share a target. Its addresses
		// belong in the additional section once.
		Qname: "mx.example.org.", Qtype: dns.TypeMX,
		Answer: []dns.RR{
			test.MX("mx.example.org. 1800 IN MX 10 host.example.org."),
			test.MX("mx.example.org. 1800 IN MX 20 host.example.org."),
		},
		Ns: additionalAuth,
		Extra: []dns.RR{
			test.A("host.example.org. 1800 IN A 192.0.2.10"),
			test.AAAA("host.example.org. 1800 IN AAAA 2001:db8::10"),
		},
	},
	{
		// SRV runs through the same additional processing.
		Qname: "srv.example.org.", Qtype: dns.TypeSRV,
		Answer: []dns.RR{
			test.SRV("srv.example.org. 1800 IN SRV 10 50 8080 host.example.org."),
			test.SRV("srv.example.org. 1800 IN SRV 20 50 8080 host.example.org."),
		},
		Ns: additionalAuth,
		Extra: []dns.RR{
			test.A("host.example.org. 1800 IN A 192.0.2.10"),
			test.AAAA("host.example.org. 1800 IN AAAA 2001:db8::10"),
		},
	},
	{
		// Distinct targets must each still be resolved.
		Qname: "two.example.org.", Qtype: dns.TypeMX,
		Answer: []dns.RR{
			test.MX("two.example.org. 1800 IN MX 10 host.example.org."),
			test.MX("two.example.org. 1800 IN MX 20 other.example.org."),
		},
		Ns: additionalAuth,
		Extra: []dns.RR{
			test.A("host.example.org. 1800 IN A 192.0.2.10"),
			test.AAAA("host.example.org. 1800 IN AAAA 2001:db8::10"),
			test.A("other.example.org. 1800 IN A 192.0.2.20"),
			test.AAAA("other.example.org. 1800 IN AAAA 2001:db8::20"),
		},
	},
}

func TestAdditionalSectionDeduplication(t *testing.T) {
	zone, err := Parse(strings.NewReader(dbAdditionalExample), testAdditionalOrigin, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{testAdditionalOrigin: zone}, Names: []string{testAdditionalOrigin}}}
	ctx := context.TODO()

	for _, tc := range additionalTestCases {
		m := tc.Msg()

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := fm.ServeDNS(ctx, rec, m)
		if err != nil {
			t.Errorf("Expected no error for %q/%d, got %v", tc.Qname, tc.Qtype, err)
			continue
		}

		resp := rec.Msg
		if err := test.SortAndCheck(resp, tc); err != nil {
			t.Errorf("Test %q/%d: %v", tc.Qname, tc.Qtype, err)
		}
	}
}

func TestAdditionalSectionDeduplicationMixedCase(t *testing.T) {
	// SRV targets are not lowercased on insert, so one target can reach additional
	// processing under two spellings. The zone's tree matches names case-insensitively,
	// so both resolve to the same addresses and those belong in the response once.
	zone, err := Parse(strings.NewReader(dbAdditionalExample), testAdditionalOrigin, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	fm := File{Next: test.ErrorHandler(), Zones: Zones{Z: map[string]*Zone{testAdditionalOrigin: zone}, Names: []string{testAdditionalOrigin}}}
	ctx := context.TODO()

	m := new(dns.Msg)
	m.SetQuestion("mixed.example.org.", dns.TypeSRV)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := fm.ServeDNS(ctx, rec, m); err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	resp := rec.Msg
	if len(resp.Answer) != 2 {
		t.Fatalf("Expected 2 answers, got %d", len(resp.Answer))
	}
	if len(resp.Extra) != 2 {
		t.Errorf("Expected 2 additional records (one A, one AAAA), got %d: %v", len(resp.Extra), resp.Extra)
	}
}

const testAdditionalOrigin = "example.org."

const dbAdditionalExample = `
$TTL 30M
$ORIGIN example.org.
@	IN SOA	ns.example.org. admin.example.org. (
			2024010100 ; serial
			14400      ; refresh (4 hours)
			3600       ; retry (1 hour)
			604800     ; expire (1 week)
			14400      ; minimum (4 hours)
			)
	IN NS	ns.example.org.

ns		IN A	192.0.2.1

; The target shared by the records below.
host		IN A	192.0.2.10
		IN AAAA	2001:db8::10

; A second, distinct target.
other		IN A	192.0.2.20
		IN AAAA	2001:db8::20

; Two MX records differing only in preference, pointing at one target.
mx		IN MX	10 host.example.org.
		IN MX	20 host.example.org.

; Two SRV records pointing at one target.
srv		IN SRV	10 50 8080 host.example.org.
		IN SRV	20 50 8080 host.example.org.

; The same target, spelled differently. SRV targets are not normalized on insert.
mixed		IN SRV	10 50 8080 host.example.org.
		IN SRV	20 50 8080 HOST.EXAMPLE.ORG.

; Two MX records with distinct targets.
two		IN MX	10 host.example.org.
		IN MX	20 other.example.org.
`
