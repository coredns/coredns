package siit

import (
	"context"
	"net"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dns64"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// emptyBackend always answers NOERROR/no-data for whatever it's asked.
type emptyBackend struct{}

func (emptyBackend) Name() string { return "empty" }
func (emptyBackend) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Rcode = dns.RcodeSuccess
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

// reentrantUpstream re-invokes top.ServeDNS for every Lookup call, exactly
// as CoreDNS's real Upstream/lookup plugin re-enters the whole server. It
// fails the test if the recursion depth exceeds a small bound, so a
// regression shows up as a test failure instead of a hang/stack overflow.
type reentrantUpstream struct {
	t     *testing.T
	top   plugin.Handler
	calls *int
	maxOK int
}

func (u reentrantUpstream) Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error) {
	*u.calls++
	if *u.calls > u.maxOK {
		u.t.Fatalf("recursion exceeded bound: %d internal lookups (siit/dns64 re-entering each other)", *u.calls)
	}
	m := new(dns.Msg)
	m.SetQuestion(name, typ)
	rec := dnstest.NewRecorder(state.W)
	_, err := u.top.ServeDNS(ctx, rec, m)
	return rec.Msg, err
}

// TestSIITDNS64NoRecursion reproduces the reported handler order
// (dns64 -> siit -> empty backend) and asserts the exchange completes
// without unbounded internal recursion. It's parameterized over both the
// IPv6-client / default-dns64 path and the IPv4-client / allow_ipv4 path,
// since they exercise different branches of dns64's interception logic and
// a fixed ResponseWriter family would silently only cover one of them.
func TestSIITDNS64NoRecursion(t *testing.T) {
	_, prefix, _ := net.ParseCIDR("64:ff9b::/96")

	cases := []struct {
		name      string
		rw        dns.ResponseWriter
		allowIPv4 bool
	}{
		{"IPv6 client, default dns64", &test.ResponseWriter6{}, false},
		{"IPv4 client, allow_ipv4", &test.ResponseWriter{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0

			s := &SIIT{IPv6Prefix: prefix, Next: emptyBackend{}}
			d := &dns64.DNS64{Prefix: prefix, AllowIPv4: tc.allowIPv4, Next: s}

			up := reentrantUpstream{t: t, top: d, calls: &calls, maxOK: 4}
			s.Upstream = up
			d.Upstream = up

			r := new(dns.Msg)
			r.SetQuestion("example.org.", dns.TypeA)

			rec := dnstest.NewRecorder(tc.rw)
			rc, err := d.ServeDNS(context.Background(), rec, r)
			if err != nil {
				t.Fatalf("ServeDNS returned error: %v", err)
			}
			if rc != dns.RcodeSuccess {
				t.Fatalf("unexpected rcode: %v", rc)
			}
			if calls != 2 {
				t.Fatalf("expected exactly 2 internal lookups (siit's AAAA lookup + dns64's nested A lookup), got %d", calls)
			}
		})
	}
}
