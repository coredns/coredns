package file

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestAliasToDelegation(t *testing.T) {
	zone, err := Parse(strings.NewReader(dbAliasDelegation), "example.org.", "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	fm := File{
		Next: test.ErrorHandler(),
		Zones: Zones{
			Z:     map[string]*Zone{"example.org.": zone},
			Names: []string{"example.org."},
		},
	}

	tests := []struct {
		name          string
		qname         string
		qtype         uint16
		authoritative bool
		answer        []dns.RR
		authority     []dns.RR
		additional    []dns.RR
	}{
		{
			name:          "CNAME to delegated child",
			qname:         "alias.example.org.",
			qtype:         dns.TypeA,
			authoritative: true,
			answer: []dns.RR{
				test.CNAME("alias.example.org. 300 IN CNAME host.child.example.org."),
			},
			authority: []dns.RR{
				test.NS("child.example.org. 300 IN NS ns.child.example.org."),
			},
			additional: []dns.RR{
				test.A("ns.child.example.org. 300 IN A 192.0.2.2"),
				test.AAAA("ns.child.example.org. 300 IN AAAA 2001:db8::2"),
			},
		},
		{
			name:          "CNAME chain to delegated child",
			qname:         "chain.example.org.",
			qtype:         dns.TypeA,
			authoritative: true,
			answer: []dns.RR{
				test.CNAME("alias.example.org. 300 IN CNAME host.child.example.org."),
				test.CNAME("chain.example.org. 300 IN CNAME alias.example.org."),
			},
			authority: []dns.RR{
				test.NS("child.example.org. 300 IN NS ns.child.example.org."),
			},
			additional: []dns.RR{
				test.A("ns.child.example.org. 300 IN A 192.0.2.2"),
				test.AAAA("ns.child.example.org. 300 IN AAAA 2001:db8::2"),
			},
		},
		{
			name:          "wildcard CNAME to delegated child",
			qname:         "name.wild.example.org.",
			qtype:         dns.TypeA,
			authoritative: true,
			answer: []dns.RR{
				test.CNAME("name.wild.example.org. 300 IN CNAME host.child.example.org."),
			},
			authority: []dns.RR{
				test.NS("child.example.org. 300 IN NS ns.child.example.org."),
			},
			additional: []dns.RR{
				test.A("ns.child.example.org. 300 IN A 192.0.2.2"),
				test.AAAA("ns.child.example.org. 300 IN AAAA 2001:db8::2"),
			},
		},
		{
			name:          "DNAME to delegated child",
			qname:         "host.mapped.example.org.",
			qtype:         dns.TypeA,
			authoritative: true,
			answer: []dns.RR{
				test.CNAME("host.mapped.example.org. 300 IN CNAME host.example.org."),
				test.DNAME("mapped.example.org. 300 IN DNAME example.org."),
			},
			authority: []dns.RR{
				test.NS("host.example.org. 300 IN NS ns.host.example.org."),
			},
			additional: []dns.RR{
				test.A("ns.host.example.org. 300 IN A 192.0.2.3"),
				test.AAAA("ns.host.example.org. 300 IN AAAA 2001:db8::3"),
			},
		},
		{
			name:          "DNAME NS query to delegated child",
			qname:         "host.mapped.example.org.",
			qtype:         dns.TypeNS,
			authoritative: true,
			answer: []dns.RR{
				test.CNAME("host.mapped.example.org. 300 IN CNAME host.example.org."),
				test.DNAME("mapped.example.org. 300 IN DNAME example.org."),
			},
			authority: []dns.RR{
				test.NS("host.example.org. 300 IN NS ns.host.example.org."),
			},
			additional: []dns.RR{
				test.A("ns.host.example.org. 300 IN A 192.0.2.3"),
				test.AAAA("ns.host.example.org. 300 IN AAAA 2001:db8::3"),
			},
		},
		{
			name:          "direct delegated child",
			qname:         "host.child.example.org.",
			qtype:         dns.TypeA,
			authoritative: false,
			authority: []dns.RR{
				test.NS("child.example.org. 300 IN NS ns.child.example.org."),
			},
			additional: []dns.RR{
				test.A("ns.child.example.org. 300 IN A 192.0.2.2"),
				test.AAAA("ns.child.example.org. 300 IN AAAA 2001:db8::2"),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := new(dns.Msg)
			req.SetQuestion(tc.qname, tc.qtype)
			rec := dnstest.NewRecorder(&test.ResponseWriter{})

			if _, err := fm.ServeDNS(context.Background(), rec, req); err != nil {
				t.Fatalf("ServeDNS() error = %v", err)
			}

			want := test.Case{
				Qname:  tc.qname,
				Qtype:  tc.qtype,
				Answer: tc.answer,
				Ns:     tc.authority,
				Extra:  tc.additional,
			}
			if err := test.SortAndCheck(rec.Msg, want); err != nil {
				t.Error(err)
			}
			if rec.Msg.Authoritative != tc.authoritative {
				t.Errorf("Authoritative = %t, want %t", rec.Msg.Authoritative, tc.authoritative)
			}
		})
	}
}

const dbAliasDelegation = `
$TTL 300
$ORIGIN example.org.
@	IN SOA	ns.example.org. admin.example.org. 1 3600 600 86400 300
	IN NS	ns.example.org.
ns	IN A	192.0.2.1

alias	IN CNAME	host.child.example.org.
chain	IN CNAME	alias.example.org.
*.wild	IN CNAME	host.child.example.org.
child	IN NS	ns.child.example.org.
ns.child	IN A	192.0.2.2
		IN AAAA	2001:db8::2

mapped	IN DNAME	example.org.
host	IN NS	ns.host.example.org.
ns.host	IN A	192.0.2.3
	IN AAAA	2001:db8::3
`

func TestSignedAliasToDelegation(t *testing.T) {
	zone, err := Parse(strings.NewReader(exampleOrgSigned), "example.org.", "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}
	if err := zone.Insert(test.CNAME("alias.example.org. 1800 IN CNAME a.delegated.example.org.")); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if err := zone.Insert(test.CNAME("ds-alias.example.org. 1800 IN CNAME delegated.example.org.")); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	fm := File{
		Next: test.ErrorHandler(),
		Zones: Zones{
			Z:     map[string]*Zone{"example.org.": zone},
			Names: []string{"example.org."},
		},
	}

	referral := secureDelegationCase(t, "a.delegated.example.org.", dns.TypeTXT)
	dsAtCut := secureDelegationCase(t, "delegated.example.org.", dns.TypeDS)
	tests := []struct {
		name       string
		qname      string
		qtype      uint16
		answer     []dns.RR
		authority  []dns.RR
		additional []dns.RR
	}{
		{
			name:  "signed partial referral",
			qname: "alias.example.org.",
			qtype: dns.TypeA,
			answer: []dns.RR{
				test.CNAME("alias.example.org. 1800 IN CNAME a.delegated.example.org."),
			},
			authority:  referral.Ns,
			additional: referral.Extra,
		},
		{
			name:  "DS at zone cut remains authoritative",
			qname: "ds-alias.example.org.",
			qtype: dns.TypeDS,
			answer: append(
				append([]dns.RR{}, dsAtCut.Answer...),
				test.CNAME("ds-alias.example.org. 1800 IN CNAME delegated.example.org."),
			),
			authority: dsAtCut.Ns,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := new(dns.Msg)
			req.SetQuestion(tc.qname, tc.qtype)
			req.SetEdns0(4096, true)
			rec := dnstest.NewRecorder(&test.ResponseWriter{})

			if _, err := fm.ServeDNS(context.Background(), rec, req); err != nil {
				t.Fatalf("ServeDNS() error = %v", err)
			}

			want := test.Case{
				Qname:  tc.qname,
				Qtype:  tc.qtype,
				Do:     true,
				Answer: tc.answer,
				Ns:     tc.authority,
				Extra:  tc.additional,
			}
			if err := test.SortAndCheck(rec.Msg, want); err != nil {
				t.Error(err)
			}
			if !rec.Msg.Authoritative {
				t.Error("Authoritative = false, want true")
			}
		})
	}
}

func secureDelegationCase(t *testing.T, qname string, qtype uint16) test.Case {
	t.Helper()
	for _, tc := range secureDelegationTestCases {
		if tc.Qname == qname && tc.Qtype == qtype {
			return tc
		}
	}
	t.Fatalf("secure delegation fixture has no %s/%s case", qname, dns.TypeToString[qtype])
	return test.Case{}
}
