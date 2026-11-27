package siit

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dns64"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

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
	if len(removed) != 1 || len(out.Answer) != 1 {
		t.Fatalf("expected 1 record removed and 1 left, got removed=%d left=%d", len(removed), len(out.Answer))
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

type soaUpstream struct{ soa dns.RR }

func (u soaUpstream) Lookup(_ context.Context, _ request.Request, name string, typ uint16) (*dns.Msg, error) {
	if typ != dns.TypeSOA {
		return nil, fmt.Errorf("unexpected lookup type %d", typ)
	}
	m := new(dns.Msg)
	m.SetQuestion(name, typ)
	m.Ns = []dns.RR{u.soa} // what a non-apex NODATA looks like
	return m, nil
}

// A positive authoritative answer carries NS (not SOA) in authority. Once all
// AAAA are removed, the response must be a proper NODATA with the SOA, not
// something that reads as a delegation (RFC 2308).
func TestExcludeAAAANegativeAuthority(t *testing.T) {
	const name = "www.example.com."
	reply := aaaaReply(name, test.AAAA(name+" 60 IN AAAA 2001:db8::1"))
	reply.Authoritative = true
	reply.Ns = []dns.RR{test.NS("example.com. 3600 IN NS ns1.example.com.")} // positive file response

	d := newExcludeSIIT(reply, true)
	d.Upstream = soaUpstream{test.SOA("example.com. 70 IN SOA ns1 hostmaster 1 1 1 1 1")}

	rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
	if _, err := d.ServeDNS(context.Background(), rec, aaaaQuery(name)); err != nil {
		t.Fatal(err)
	}
	if len(rec.Msg.Answer) != 0 {
		t.Fatalf("expected NODATA, got %v", rec.Msg.Answer)
	}
	if len(rec.Msg.Ns) != 1 || rec.Msg.Ns[0].Header().Rrtype != dns.TypeSOA {
		t.Fatalf("expected only an SOA in authority, got %v", rec.Msg.Ns)
	}
}

// The siit plugin can end the chain (README example), so Next may be nil.
func TestExcludeAAAANilNext(t *testing.T) {
	d := &SIIT{ExcludeAAAA: true}
	rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
	rc, err := d.ServeDNS(context.Background(), rec, aaaaQuery("example.com."))
	if err == nil || rc != dns.RcodeServerFailure {
		t.Fatalf("expected the missing-backend error, got rc=%d err=%v", rc, err)
	}
}

// aaaaOnlyBackend answers AAAA with a SIIT-mapped address, everything else NODATA.
type aaaaOnlyBackend struct{}

func (aaaaOnlyBackend) Name() string { return "aaaaonly" }
func (aaaaOnlyBackend) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	m := new(dns.Msg)
	m.SetReply(r)
	if r.Question[0].Qtype == dns.TypeAAAA {
		m.Answer = []dns.RR{test.AAAA("example.org. 60 IN AAAA 64:ff9b::192.0.2.1")}
	}
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

// With dns64 and siit sharing the default prefix, the excluded address must not
// come back through dns64's synthesis from the translated A record.
func TestExcludeAAAAWithDNS64SamePrefix(t *testing.T) {
	_, prefix, _ := net.ParseCIDR("64:ff9b::/96")
	calls := 0
	s := &SIIT{IPv6Prefix: prefix, ExcludeAAAA: true, Next: aaaaOnlyBackend{}}
	d := &dns64.DNS64{Prefix: prefix, Next: s}
	up := reentrantUpstream{t: t, top: d, calls: &calls, maxOK: 4}
	s.Upstream, d.Upstream = up, up

	q := new(dns.Msg)
	q.SetQuestion("example.org.", dns.TypeAAAA)
	rec := dnstest.NewRecorder(&test.ResponseWriter6{})
	if _, err := d.ServeDNS(context.Background(), rec, q); err != nil {
		t.Fatal(err)
	}
	if len(rec.Msg.Answer) != 0 {
		t.Fatalf("mapped AAAA leaked through dns64: %v", rec.Msg.Answer)
	}

	// A direct A query still gets the synthesized A record.
	s.Upstream = &fakeUpstream{t, "example.org.", aaaaReply("example.org.", test.AAAA("example.org. 60 IN AAAA 64:ff9b::192.0.2.1"))}
	qa := new(dns.Msg)
	qa.SetQuestion("example.org.", dns.TypeA)
	rec = dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
	if _, err := s.ServeDNS(context.Background(), rec, qa); err != nil {
		t.Fatal(err)
	}
	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected the A record, got %v", rec.Msg.Answer)
	}
}

const zoneSOA = "example.org. 3600 IN SOA ns1.example.org. hostmaster.example.org. 1 7200 3600 1209600 60"

// zoneBackend mimics a file-backed zone: positive answers carry the NS set in
// authority, negative ones (and SOA queries below the apex) carry the SOA.
type zoneBackend struct{ aaaa, a []dns.RR }

func (zoneBackend) Name() string { return "zone" }
func (z zoneBackend) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	switch r.Question[0].Qtype {
	case dns.TypeAAAA:
		m.Answer = z.aaaa
	case dns.TypeA:
		m.Answer = z.a
	}
	if len(m.Answer) > 0 {
		m.Ns = []dns.RR{test.NS("example.org. 3600 IN NS ns1.example.org.")}
	} else {
		m.Ns = []dns.RR{test.SOA(zoneSOA)}
	}
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

func dns64Chain(t *testing.T, dns64Prefix string, backend plugin.Handler) *dns64.DNS64 {
	t.Helper()
	_, siitPrefix, _ := net.ParseCIDR("64:ff9b::/96")
	_, d64Prefix, _ := net.ParseCIDR(dns64Prefix)
	calls := 0
	s := &SIIT{IPv6Prefix: siitPrefix, ExcludeAAAA: true, Next: backend}
	d := &dns64.DNS64{Prefix: d64Prefix, Next: s}
	up := reentrantUpstream{t: t, top: d, calls: &calls, maxOK: 6}
	s.Upstream, d.Upstream = up, up
	return d
}

func aaaaAnswers(m *dns.Msg) []string {
	var out []string
	for _, rr := range m.Answer {
		if a, ok := rr.(*dns.AAAA); ok {
			out = append(out, a.AAAA.String())
		}
	}
	return out
}

// A name with a native A record and the matching mapped AAAA: dns64 would
// derive the excluded address from the A record. It must not come back.
func TestExcludeAAAAWithDNS64NativeA(t *testing.T) {
	b := zoneBackend{
		aaaa: []dns.RR{test.AAAA("example.org. 60 IN AAAA 64:ff9b::192.0.2.42")},
		a:    []dns.RR{test.A("example.org. 60 IN A 192.0.2.42")},
	}
	d := dns64Chain(t, "64:ff9b::/96", b)

	rec := dnstest.NewRecorder(&test.ResponseWriter6{})
	if _, err := d.ServeDNS(context.Background(), rec, aaaaQuery("example.org.")); err != nil {
		t.Fatal(err)
	}
	if got := aaaaAnswers(rec.Msg); len(got) != 0 {
		t.Fatalf("excluded address came back through dns64: %v", got)
	}
	if len(rec.Msg.Ns) != 1 {
		t.Fatalf("expected only the SOA in authority, got %v", rec.Msg.Ns)
	}
	soa, ok := rec.Msg.Ns[0].(*dns.SOA)
	if !ok || soa.Hdr.Ttl != 60 {
		t.Fatalf("expected SOA with negative TTL 60, got %v", rec.Msg.Ns[0])
	}
}

// A name with only an A record still gets normal dns64 synthesis.
func TestExcludeAAAAWithDNS64AOnly(t *testing.T) {
	b := zoneBackend{a: []dns.RR{test.A("example.org. 60 IN A 192.0.2.42")}}
	d := dns64Chain(t, "64:ff9b::/96", b)

	rec := dnstest.NewRecorder(&test.ResponseWriter6{})
	if _, err := d.ServeDNS(context.Background(), rec, aaaaQuery("example.org.")); err != nil {
		t.Fatal(err)
	}
	got := aaaaAnswers(rec.Msg)
	if len(got) != 1 || got[0] != "64:ff9b::c000:22a" {
		t.Fatalf("expected dns64 synthesis for an A-only name, got %v", got)
	}
}

// With a distinct dns64 prefix the synthesized address differs from the
// excluded one, so it is kept.
func TestExcludeAAAAWithDNS64DifferentPrefix(t *testing.T) {
	b := zoneBackend{
		aaaa: []dns.RR{test.AAAA("example.org. 60 IN AAAA 64:ff9b::192.0.2.42")},
		a:    []dns.RR{test.A("example.org. 60 IN A 192.0.2.42")},
	}
	d := dns64Chain(t, "64:dead::/96", b)

	rec := dnstest.NewRecorder(&test.ResponseWriter6{})
	if _, err := d.ServeDNS(context.Background(), rec, aaaaQuery("example.org.")); err != nil {
		t.Fatal(err)
	}
	got := aaaaAnswers(rec.Msg)
	if len(got) != 1 || got[0] != "64:dead::c000:22a" {
		t.Fatalf("expected the address under the dns64 prefix, got %v", got)
	}
}

// upstream recording which name the SOA was asked for.
type namedSOAUpstream struct {
	soa  dns.RR
	name *string
}

func (u namedSOAUpstream) Lookup(_ context.Context, _ request.Request, name string, typ uint16) (*dns.Msg, error) {
	if typ != dns.TypeSOA {
		return nil, fmt.Errorf("unexpected lookup type %d", typ)
	}
	*u.name = name
	m := new(dns.Msg)
	m.SetQuestion(name, typ)
	if name == "mapped.example.org." { // the alias itself would answer with its CNAME
		m.Ns = []dns.RR{u.soa}
	} else {
		m.Answer = []dns.RR{test.CNAME("alias.example.org. 60 IN CNAME mapped.example.org.")}
	}
	return m, nil
}

// The SOA of a negative answer behind a CNAME comes from the end of the chain.
func TestExcludeAAAANegativeAuthorityCNAME(t *testing.T) {
	const alias = "alias.example.org."
	reply := aaaaReply(alias,
		test.CNAME(alias+" 60 IN CNAME mapped.example.org."),
		test.AAAA("mapped.example.org. 60 IN AAAA 2001:db8::1"),
	)
	reply.Ns = []dns.RR{test.NS("example.org. 3600 IN NS ns1.example.org.")}

	var asked string
	d := newExcludeSIIT(reply, true)
	d.Upstream = namedSOAUpstream{test.SOA(zoneSOA), &asked}

	rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
	if _, err := d.ServeDNS(context.Background(), rec, aaaaQuery(alias)); err != nil {
		t.Fatal(err)
	}
	if asked != "mapped.example.org." {
		t.Errorf("SOA must be looked up for the CNAME target, asked for %q", asked)
	}
	if len(rec.Msg.Answer) != 1 || rec.Msg.Answer[0].Header().Rrtype != dns.TypeCNAME {
		t.Errorf("expected the CNAME to be kept, got %v", rec.Msg.Answer)
	}
	if len(rec.Msg.Ns) != 1 || rec.Msg.Ns[0].Header().Rrtype != dns.TypeSOA {
		t.Errorf("expected the zone SOA in authority, got %v", rec.Msg.Ns)
	}
}

func TestTerminalNameLoop(t *testing.T) {
	loop := []dns.RR{
		test.CNAME("a.example.org. 60 IN CNAME b.example.org."),
		test.CNAME("b.example.org. 60 IN CNAME a.example.org."),
	}
	// must terminate, and end at a name that is part of the loop
	if got := terminalName("a.example.org.", loop); got != "a.example.org." && got != "b.example.org." {
		t.Errorf("unexpected terminal name %q for a CNAME loop", got)
	}
}

// Negative answers advertise min(SOA TTL, SOA MINIMUM) (RFC 2308 section 3),
// whether the SOA was looked up or already in the response, and the record the
// upstream handed out is not altered.
func TestExcludeAAAANegativeSOATTL(t *testing.T) {
	const name = "example.org."

	t.Run("looked up", func(t *testing.T) {
		upstreamSOA := test.SOA(zoneSOA)
		reply := aaaaReply(name, test.AAAA(name+" 60 IN AAAA 2001:db8::1"))
		reply.Ns = []dns.RR{test.NS("example.org. 3600 IN NS ns1.example.org.")}
		d := newExcludeSIIT(reply, true)
		d.Upstream = soaUpstream{upstreamSOA}

		rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
		if _, err := d.ServeDNS(context.Background(), rec, aaaaQuery(name)); err != nil {
			t.Fatal(err)
		}
		if ttl := rec.Msg.Ns[0].Header().Ttl; ttl != 60 {
			t.Errorf("expected negative SOA TTL 60, got %d", ttl)
		}
		if ttl := upstreamSOA.Header().Ttl; ttl != 3600 {
			t.Errorf("upstream SOA must keep TTL 3600, got %d", ttl)
		}
	})

	t.Run("already in response", func(t *testing.T) {
		inResp := test.SOA(zoneSOA)
		reply := aaaaReply(name, test.AAAA(name+" 60 IN AAAA 2001:db8::1"))
		reply.Ns = []dns.RR{inResp}
		d := newExcludeSIIT(reply, true)

		rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
		if _, err := d.ServeDNS(context.Background(), rec, aaaaQuery(name)); err != nil {
			t.Fatal(err)
		}
		if ttl := rec.Msg.Ns[0].Header().Ttl; ttl != 60 {
			t.Errorf("expected negative SOA TTL 60, got %d", ttl)
		}
		if ttl := inResp.Header().Ttl; ttl != 3600 {
			t.Errorf("the original response must not be altered, TTL is now %d", ttl)
		}
	})
}
