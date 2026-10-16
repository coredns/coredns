package dnslkg

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// nextHandler is a stub upstream that returns a preset reply (or error).
type nextHandler struct {
	answer []dns.RR
	ns     []dns.RR
	rcode  int
	err    error
	delay  time.Duration
}

func (n *nextHandler) Name() string { return "next" }

func (n *nextHandler) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if n.delay > 0 {
		time.Sleep(n.delay)
	}
	if n.err != nil {
		return n.rcode, n.err
	}
	m := new(dns.Msg)
	m.SetReply(r)
	m.Rcode = n.rcode
	m.Answer = n.answer
	m.Ns = n.ns
	w.WriteMsg(m)
	return n.rcode, nil
}

func newTestPlugin(t *testing.T, next plugin.Handler) *DnsLKG {
	t.Helper()
	return &DnsLKG{
		Next:    next,
		store:   newMemStore(0, 0),
		ttl:     defaultTTL,
		fb:      allFallbacks(),
		matcher: newNameMatcher(nil),
	}
}

func query(qname string, qtype uint16) *dns.Msg {
	r := new(dns.Msg)
	r.SetQuestion(qname, qtype)
	return r
}

func TestServeStoresGoodAnswer(t *testing.T) {
	next := &nextHandler{answer: []dns.RR{test.A("example.org. 300 IN A 127.0.0.1")}}
	d := newTestPlugin(t, next)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := d.ServeDNS(context.TODO(), rec, query("example.org.", dns.TypeA)); err != nil {
		t.Fatalf("ServeDNS: %v", err)
	}
	if rec.Msg == nil || len(rec.Msg.Answer) != 1 {
		t.Fatalf("Expected the good answer to be passed through, got %v", rec.Msg)
	}

	got, _, err := d.store.Get("example.org.", dns.TypeA)
	if err != nil {
		t.Fatalf("Store get: %v", err)
	}
	if got == nil {
		t.Fatal("Expected the good answer to be stored")
	}
}

func TestServeLKGOnNXDOMAIN(t *testing.T) {
	// Prime the store with a good answer.
	good := &nextHandler{answer: []dns.RR{test.A("example.org. 300 IN A 127.0.0.1")}}
	d := newTestPlugin(t, good)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	d.ServeDNS(context.TODO(), rec, query("example.org.", dns.TypeA))

	// Upstream now (mis)configured to return NXDOMAIN.
	d.Next = &nextHandler{rcode: dns.RcodeNameError}
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := d.ServeDNS(context.TODO(), rec, query("example.org.", dns.TypeA)); err != nil {
		t.Fatalf("ServeDNS: %v", err)
	}
	if rec.Msg.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected NOERROR from LKG, got %s", dns.RcodeToString[rec.Msg.Rcode])
	}
	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("Expected 1 LKG answer, got %d", len(rec.Msg.Answer))
	}
	if rec.Msg.Answer[0].(*dns.A).A.String() != "127.0.0.1" {
		t.Errorf("Unexpected LKG answer: %v", rec.Msg.Answer[0])
	}
	// Served TTL should be the configured value.
	if rec.Msg.Answer[0].Header().Ttl != uint32(defaultTTL.Seconds()) {
		t.Errorf("Expected served TTL %d, got %d", uint32(defaultTTL.Seconds()), rec.Msg.Answer[0].Header().Ttl)
	}
}

func TestServeLKGOnNODATA(t *testing.T) {
	good := &nextHandler{answer: []dns.RR{test.A("example.org. 300 IN A 127.0.0.1")}}
	d := newTestPlugin(t, good)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	d.ServeDNS(context.TODO(), rec, query("example.org.", dns.TypeA))

	// NODATA = NOERROR with an SOA in authority and no answer.
	soa := test.SOA("example.org. 300 IN SOA ns.example.org. hostmaster.example.org. 1 2 3 4 5")
	d.Next = &nextHandler{rcode: dns.RcodeSuccess, ns: []dns.RR{soa}}
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	d.ServeDNS(context.TODO(), rec, query("example.org.", dns.TypeA))

	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("Expected 1 LKG answer on NODATA, got %d", len(rec.Msg.Answer))
	}
}

func TestServeLKGOnUpstreamError(t *testing.T) {
	good := &nextHandler{answer: []dns.RR{test.A("example.org. 300 IN A 127.0.0.1")}}
	d := newTestPlugin(t, good)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	d.ServeDNS(context.TODO(), rec, query("example.org.", dns.TypeA))

	d.Next = &nextHandler{rcode: dns.RcodeServerFailure, err: errors.New("boom")}
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := d.ServeDNS(context.TODO(), rec, query("example.org.", dns.TypeA)); err != nil {
		t.Fatalf("Expected LKG to mask the error, got %v", err)
	}
	if rec.Msg == nil || len(rec.Msg.Answer) != 1 {
		t.Fatalf("Expected LKG answer on upstream error, got %v", rec.Msg)
	}
}

func TestNXDOMAINPassThroughWithoutLKG(t *testing.T) {
	d := newTestPlugin(t, &nextHandler{rcode: dns.RcodeNameError})
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := d.ServeDNS(context.TODO(), rec, query("nope.org.", dns.TypeA)); err != nil {
		t.Fatalf("ServeDNS: %v", err)
	}
	if rec.Msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN to pass through, got %s", dns.RcodeToString[rec.Msg.Rcode])
	}
}

func TestNODATADifferentTypeNotServed(t *testing.T) {
	// Store a good A answer.
	d := newTestPlugin(t, &nextHandler{answer: []dns.RR{test.A("example.org. 300 IN A 127.0.0.1")}})
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	d.ServeDNS(context.TODO(), rec, query("example.org.", dns.TypeA))

	// A genuine NODATA for AAAA (never stored) must pass through, not serve the A.
	soa := test.SOA("example.org. 300 IN SOA ns.example.org. hostmaster.example.org. 1 2 3 4 5")
	d.Next = &nextHandler{rcode: dns.RcodeSuccess, ns: []dns.RR{soa}}
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	d.ServeDNS(context.TODO(), rec, query("example.org.", dns.TypeAAAA))

	if len(rec.Msg.Answer) != 0 {
		t.Errorf("Expected NODATA pass-through for AAAA, got %d answers", len(rec.Msg.Answer))
	}
}

func TestShouldTrack(t *testing.T) {
	tests := []struct {
		name    string
		include []string
		exclude []string
		qname   string
		want    bool
	}{
		{"no patterns", nil, nil, "a.example.org.", true},
		{"include subtree match", []string{"*.example.org"}, nil, "a.example.org.", true},
		{"include no match", []string{"*.example.org"}, nil, "a.example.com.", false},
		{"include apex only", []string{"example.org"}, nil, "example.org.", true},
		{"include apex excludes subdomain", []string{"example.org"}, nil, "a.example.org.", false},
		{"exclude subtree match", nil, []string{"*.internal"}, "a.internal.", false},
		{"exclude no match", nil, []string{"*.internal"}, "a.example.org.", true},
		{"most specific exclude beats broad include", []string{"*.example.org"}, []string{"bad.example.org"}, "bad.example.org.", false},
		{"most specific include beats broad exclude", []string{"api.internal.example.com"}, []string{"*.internal.example.com"}, "api.internal.example.com.", true},
		{"nested exception subtree still excluded", []string{"*.example.com"}, []string{"*.internal.example.com"}, "db.internal.example.com.", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var rules []rule
			for _, p := range tc.include {
				rules = append(rules, mustRule(t, p, true))
			}
			for _, p := range tc.exclude {
				rules = append(rules, mustRule(t, p, false))
			}
			d := &DnsLKG{matcher: newNameMatcher(rules)}
			if got := d.shouldTrack(tc.qname); got != tc.want {
				t.Errorf("ShouldTrack(%q) = %v, want %v", tc.qname, got, tc.want)
			}
		})
	}
}

func mustRule(t *testing.T, pattern string, include bool) rule {
	t.Helper()
	r, err := parseRule(pattern, include)
	if err != nil {
		t.Fatalf("Bad pattern %q: %v", pattern, err)
	}
	return r
}

func TestServeDNSUntrackedBypasses(t *testing.T) {
	next := &nextHandler{rcode: dns.RcodeNameError}
	d := newTestPlugin(t, next)
	d.matcher = newNameMatcher([]rule{mustRule(t, "*.only.this", true)})

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	// Prime an LKG entry for a name we won't query as tracked.
	d.store.Put("other.org.", dns.TypeA, msgWith("other.org.", dns.TypeA, test.A("other.org. 300 IN A 1.2.3.4")))

	d.ServeDNS(context.TODO(), rec, query("other.org.", dns.TypeA))
	if rec.Msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected untracked name to bypass LKG and return NXDOMAIN, got %s", dns.RcodeToString[rec.Msg.Rcode])
	}
}

// TestFallbackOnGating verifies that a trigger absent from fallback_on causes
// the failure to pass through untouched, while an enabled trigger serves LKG.
func TestFallbackOnGating(t *testing.T) {
	good := &nextHandler{answer: []dns.RR{test.A("example.org. 300 IN A 127.0.0.1")}}
	d := newTestPlugin(t, good)
	// Only NODATA should trigger a fallback.
	d.fb = fallbackSet{nodata: true}

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	d.ServeDNS(context.TODO(), rec, query("example.org.", dns.TypeA))

	// NXDOMAIN is not an enabled trigger: it must pass through.
	d.Next = &nextHandler{rcode: dns.RcodeNameError}
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	d.ServeDNS(context.TODO(), rec, query("example.org.", dns.TypeA))
	if rec.Msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN pass-through when nxdomain trigger disabled, got %s", dns.RcodeToString[rec.Msg.Rcode])
	}

	// NODATA is enabled: it must serve the stored answer.
	soa := test.SOA("example.org. 300 IN SOA ns.example.org. hostmaster.example.org. 1 2 3 4 5")
	d.Next = &nextHandler{rcode: dns.RcodeSuccess, ns: []dns.RR{soa}}
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	d.ServeDNS(context.TODO(), rec, query("example.org.", dns.TypeA))
	if len(rec.Msg.Answer) != 1 {
		t.Errorf("Expected LKG answer on NODATA, got %d answers", len(rec.Msg.Answer))
	}
}

// TestFallbackTimeoutServesLKG verifies that a slow upstream causes the stored
// answer to be served once the soft deadline is exceeded.
func TestFallbackTimeoutServesLKG(t *testing.T) {
	// Prime the store with a fast good answer.
	d := newTestPlugin(t, &nextHandler{answer: []dns.RR{test.A("example.org. 300 IN A 127.0.0.1")}})
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	d.ServeDNS(context.TODO(), rec, query("example.org.", dns.TypeA))

	// Now make the upstream slow and enable the soft deadline.
	d.fallbackTimeout = 20 * time.Millisecond
	d.Next = &nextHandler{answer: []dns.RR{test.A("example.org. 300 IN A 9.9.9.9")}, delay: 500 * time.Millisecond}

	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	start := time.Now()
	d.ServeDNS(context.TODO(), rec, query("example.org.", dns.TypeA))
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("Expected fast LKG fallback, took %v", elapsed)
	}
	if len(rec.Msg.Answer) != 1 || rec.Msg.Answer[0].(*dns.A).A.String() != "127.0.0.1" {
		t.Fatalf("Expected stored LKG answer 127.0.0.1, got %v", rec.Msg.Answer)
	}
}

// TestFallbackTimeoutWaitsWithoutLKG verifies that with no stored answer the
// soft deadline does not fabricate a response but waits for the upstream.
func TestFallbackTimeoutWaitsWithoutLKG(t *testing.T) {
	d := newTestPlugin(t, &nextHandler{answer: []dns.RR{test.A("fresh.org. 300 IN A 8.8.8.8")}, delay: 30 * time.Millisecond})
	d.fallbackTimeout = 10 * time.Millisecond

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	d.ServeDNS(context.TODO(), rec, query("fresh.org.", dns.TypeA))

	if len(rec.Msg.Answer) != 1 || rec.Msg.Answer[0].(*dns.A).A.String() != "8.8.8.8" {
		t.Fatalf("Expected the real upstream answer when no LKG exists, got %v", rec.Msg.Answer)
	}
}
