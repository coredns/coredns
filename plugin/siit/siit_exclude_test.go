package siit

import (
	"context"
	"net"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/coredns/caddy"
	"github.com/miekg/dns"
)

func aaaaQuery(name string) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeAAAA)
	return m
}

func aaaaReply(name string, answer ...dns.RR) *dns.Msg {
	m := aaaaQuery(name)
	m.Response = true
	m.Answer = answer
	m.Ns = []dns.RR{test.SOA("example.com. 70 IN SOA foo bar 1 1 1 1 1")}
	return m
}

func newExcludeSIIT(reply *dns.Msg, exclude bool) *SIIT {
	_, pfx, _ := net.ParseCIDR("64:ff9b::/96")
	return &SIIT{
		Next:        &fakeHandler{reply: reply},
		IPv6Prefix:  pfx,
		Eam4:        map[string]net.IP{"2001:db8::1": net.ParseIP("192.0.2.1")},
		ExcludeAAAA: exclude,
	}
}

func TestExcludeAAAA(t *testing.T) {
	const name = "example.com."

	rrsig := func(ttl string) dns.RR {
		return test.RRSIG(name + " " + ttl + " IN RRSIG AAAA 8 2 300 20300101000000 20200101000000 1234 example.com. AAAA")
	}

	tests := []struct {
		name      string
		reply     *dns.Msg
		exclude   bool
		wantAAAA  []string // addresses expected in the answer
		wantOther int      // non-AAAA records expected in the answer
	}{
		{
			name:     "option disabled keeps everything",
			reply:    aaaaReply(name, test.AAAA(name+" 60 IN AAAA 64:ff9b::192.0.2.42"), test.AAAA(name+" 60 IN AAAA 2001:db8::1")),
			exclude:  false,
			wantAAAA: []string{"64:ff9b::c000:22a", "2001:db8::1"},
		},
		{
			name:     "prefix address is removed",
			reply:    aaaaReply(name, test.AAAA(name+" 60 IN AAAA 64:ff9b::192.0.2.42"), test.AAAA(name+" 60 IN AAAA 2001:db8:ffff::5")),
			exclude:  true,
			wantAAAA: []string{"2001:db8:ffff::5"},
		},
		{
			name:     "eam address is removed",
			reply:    aaaaReply(name, test.AAAA(name+" 60 IN AAAA 2001:db8::1"), test.AAAA(name+" 60 IN AAAA 2001:db8:ffff::5")),
			exclude:  true,
			wantAAAA: []string{"2001:db8:ffff::5"},
		},
		{
			name:     "everything mapped yields NODATA",
			reply:    aaaaReply(name, test.AAAA(name+" 60 IN AAAA 2001:db8::1"), test.AAAA(name+" 60 IN AAAA 64:ff9b::192.0.2.42")),
			exclude:  true,
			wantAAAA: nil,
		},
		{
			name:     "unmapped addresses are untouched",
			reply:    aaaaReply(name, test.AAAA(name+" 60 IN AAAA 2001:db8:ffff::5")),
			exclude:  true,
			wantAAAA: []string{"2001:db8:ffff::5"},
		},
		{
			name: "CNAME and RRSIG of a modified RRset handling",
			reply: aaaaReply(name,
				test.CNAME("www.example.com. 60 IN CNAME "+name),
				test.AAAA(name+" 60 IN AAAA 2001:db8::1"),
				rrsig("60"),
			),
			exclude:   true,
			wantAAAA:  nil,
			wantOther: 1, // only the CNAME survives, the now-invalid RRSIG is dropped
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newExcludeSIIT(tc.reply, tc.exclude)
			rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})

			rc, err := d.ServeDNS(context.Background(), rec, aaaaQuery(name))
			if err != nil {
				t.Fatal(err)
			}
			if rc != dns.RcodeSuccess || rec.Msg.Rcode != dns.RcodeSuccess {
				t.Fatalf("expected NOERROR, got rc=%d rcode=%d", rc, rec.Msg.Rcode)
			}

			var gotAAAA []string
			other := 0
			for _, rr := range rec.Msg.Answer {
				if a, ok := rr.(*dns.AAAA); ok {
					gotAAAA = append(gotAAAA, a.AAAA.String())
				} else {
					other++
				}
			}
			if len(gotAAAA) != len(tc.wantAAAA) {
				t.Fatalf("expected AAAA %v, got %v", tc.wantAAAA, gotAAAA)
			}
			for i := range gotAAAA {
				if gotAAAA[i] != tc.wantAAAA[i] {
					t.Errorf("expected AAAA %v, got %v", tc.wantAAAA, gotAAAA)
				}
			}
			if other != tc.wantOther {
				t.Errorf("expected %d non-AAAA answer records, got %d: %v", tc.wantOther, other, rec.Msg.Answer)
			}
			if len(rec.Msg.Ns) != 1 {
				t.Errorf("authority section (SOA) must be preserved, got %v", rec.Msg.Ns)
			}
		})
	}
}

// The response coming from the next plugin must not be mutated: it may be
// shared with a cache.
func TestExcludeAAAADoesNotMutateInput(t *testing.T) {
	const name = "example.com."
	reply := aaaaReply(name, test.AAAA(name+" 60 IN AAAA 2001:db8::1"), test.AAAA(name+" 60 IN AAAA 2001:db8:ffff::5"))
	reply.AuthenticatedData = true
	d := newExcludeSIIT(reply, true)

	out, removed := d.excludeMappedAAAA(reply)
	if removed != 1 || len(out.Answer) != 1 {
		t.Fatalf("expected 1 record removed and 1 left, got removed=%d left=%d", removed, len(out.Answer))
	}
	if len(reply.Answer) != 2 {
		t.Errorf("input message was modified: %v", reply.Answer)
	}
	if out.AuthenticatedData {
		t.Errorf("AD bit must be cleared on a modified response")
	}
}

// Only AAAA queries are filtered: an A query must behave as before.
func TestExcludeAAAAIgnoresOtherQueryTypes(t *testing.T) {
	const name = "example.com."
	reply := new(dns.Msg)
	reply.SetQuestion(name, dns.TypeA)
	reply.Response = true
	reply.Answer = []dns.RR{test.A(name + " 60 IN A 192.0.2.1")}

	d := newExcludeSIIT(reply, true)
	rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
	q := new(dns.Msg)
	q.SetQuestion(name, dns.TypeA)

	if _, err := d.ServeDNS(context.Background(), rec, q); err != nil {
		t.Fatal(err)
	}
	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("A answer must be untouched, got %v", rec.Msg.Answer)
	}
}

// SIIT's own internal AAAA lookup (marked context) must never be filtered,
// otherwise A synthesis would stop working when exclude_aaaa is enabled.
func TestExcludeAAAASkipsSynthesisLookup(t *testing.T) {
	const name = "example.com."
	reply := aaaaReply(name, test.AAAA(name+" 60 IN AAAA 2001:db8::1"))
	d := newExcludeSIIT(reply, true)
	rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})

	if _, err := d.ServeDNS(markSynthesisLookup(context.Background()), rec, aaaaQuery(name)); err != nil {
		t.Fatal(err)
	}
	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("internal synthesis lookup must not be filtered, got %v", rec.Msg.Answer)
	}
}

// With exclude_aaaa enabled, A synthesis from mapped AAAA still works.
func TestExcludeAAAAKeepsASynthesis(t *testing.T) {
	const name = "example.com."
	initResp := new(dns.Msg)
	initResp.SetQuestion(name, dns.TypeA)
	initResp.Response = true
	initResp.Ns = []dns.RR{test.SOA("example.com. 70 IN SOA foo bar 1 1 1 1 1")}

	d := newExcludeSIIT(initResp, true)
	d.Upstream = &fakeUpstream{t, name, aaaaReply(name, test.AAAA(name+" 60 IN AAAA 2001:db8::1"))}

	rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
	q := new(dns.Msg)
	q.SetQuestion(name, dns.TypeA)
	if _, err := d.ServeDNS(context.Background(), rec, q); err != nil {
		t.Fatal(err)
	}
	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected one synthesized A record, got %v", rec.Msg.Answer)
	}
	if a, ok := rec.Msg.Answer[0].(*dns.A); !ok || !a.A.Equal(net.ParseIP("192.0.2.1")) {
		t.Errorf("expected A 192.0.2.1, got %v", rec.Msg.Answer[0])
	}
}

func TestSetupExcludeAAAA(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
		want      bool
	}{
		{`siit`, false, false},
		{"siit {\n exclude_aaaa\n}", false, true},
		{"siit {\n ipv6_prefix 64:dead::/96\n eam 10.0.0.1 64:dead::1\n exclude_aaaa\n}", false, true},
		{"siit {\n exclude_aaaa true\n}", true, false},
	}
	for i, tc := range tests {
		s, err := siitParse(caddy.NewTestController("dns", tc.input))
		if (err != nil) != tc.shouldErr {
			t.Errorf("test %d: expected error=%v, got %v", i, tc.shouldErr, err)
			continue
		}
		if err == nil && s.ExcludeAAAA != tc.want {
			t.Errorf("test %d: expected ExcludeAAAA=%v, got %v", i, tc.want, s.ExcludeAAAA)
		}
	}
}
