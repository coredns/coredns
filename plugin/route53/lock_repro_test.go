package route53

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
)

// TestServeDNSDoesNotHoldLockDuringUpstreamLookup reproduces the same bug
// class that plugin/azure fixed in PR #8447 ("plugin/azure: don't hold zMu
// across zone Lookup"), but for plugin/route53, where it is still present
// in route53.go's ServeDNS:
//
//	h.zMu.RLock()
//	m.Answer, m.Ns, m.Extra, result = hostedZone.z.Lookup(ctx, state, qname)
//	h.zMu.RUnlock()
//
// route53 zones are created with newZ.Upstream = h.upstream (see
// updateZones), so Lookup can chase a CNAME whose target is outside the
// zone via Upstream.Lookup, which re-enters the plugin chain and can block
// on real network I/O -- all while h.zMu.RLock() is held. Go's
// sync.RWMutex blocks new RLock() calls once a Lock() is pending, so one
// slow/hanging upstream lookup here stalls updateZones' periodic
// h.zMu.Lock() call, and every other query queues up behind that pending
// writer -- even for unrelated zones. This is also the direct ancestor of
// the still-open github.com/coredns/coredns/issues/6664: because Lookup
// runs *inside* the locked section, a panic inside it is caught by
// CoreDNS's per-request recover() in core/dnsserver/server.go but never
// reaches h.zMu.RUnlock(), permanently wedging the mutex.
func TestServeDNSDoesNotHoldLockDuringUpstreamLookup(t *testing.T) {
	const timeout = 5 * time.Second

	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseHandler()

	cfg := &dnsserver.Config{
		Zone: ".",
		Plugin: []plugin.Plugin{
			func(plugin.Handler) plugin.Handler {
				return plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
					close(entered)
					<-release
					m := new(dns.Msg)
					m.SetReply(r)
					m.Answer = []dns.RR{test.A("external.target. 300 IN A 5.6.7.8")}
					w.WriteMsg(m)
					return dns.RcodeSuccess, nil
				})
			},
		},
	}
	srv, err := dnsserver.NewServer("", []*dnsserver.Config{cfg})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), dnsserver.Key{}, srv)

	newZ := file.NewZone("example.org.", "")
	newZ.Upstream = upstream.New()
	for _, rr := range []string{
		"example.org.	300	IN	SOA	ns1.example.org. hostmaster.example.org. 1 3600 300 2419200 300",
		"cname-ext.example.org. 300 IN CNAME external.target.",
	} {
		r, _ := dns.NewRR(rr)
		newZ.Insert(r)
	}

	h := &Route53{
		Fall:      fall.Zero,
		zoneNames: []string{"example.org."},
		zones:     zones{"example.org.": {{id: "Z1", dns: "example.org.", z: newZ}}},
	}

	req := new(dns.Msg)
	req.SetQuestion("cname-ext.example.org.", dns.TypeA)

	done := make(chan struct{})
	go func() {
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		h.ServeDNS(ctx, rec, req)
		close(done)
	}()

	select {
	case <-entered:
		// The query reached the upstream handler and is now blocked on
		// release, with h.zMu.RLock() (claimed) held for the duration if
		// the bug is present.
	case <-time.After(timeout):
		t.Fatal("query never reached the upstream handler")
	}

	lockAcquired := make(chan struct{})
	go func() {
		// Stands in for the periodic zone swap updateZones performs.
		h.zMu.Lock()
		h.zones["example.org."][0].z = file.NewZone("example.org.", "")
		h.zMu.Unlock()
		close(lockAcquired)
	}()

	select {
	case <-lockAcquired:
		// h.zMu.Lock() was not blocked by the in-flight upstream lookup:
		// the fix is in place.
	case <-time.After(timeout):
		t.Fatal("h.zMu.Lock() blocked while a query awaited a slow upstream lookup (bug reproduced)")
	}

	releaseHandler()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("ServeDNS did not complete after the upstream handler was released")
	}
}
