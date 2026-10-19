package forward

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestStartFailedParse(t *testing.T) {
	for _, tc := range []struct {
		name, options string
		wantErr       bool
	}{
		{"enabled", "start_failed", false},
		{"extra argument", "start_failed true", true},
		{"health disabled before", "max_fails 0\nstart_failed", true},
		{"health disabled after", "start_failed\nmax_fails 0", true},
		{"maximum threshold", "start_failed\nmax_fails 4294967295", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseForward(caddy.NewTestController("dns", "forward . 127.0.0.1 {\n"+tc.options+"\n}"))
			if (err != nil) != tc.wantErr {
				t.Fatalf("parse error = %v, want error %v", err, tc.wantErr)
			}
		})
	}
}

func TestStartFailedHealthAndRouting(t *testing.T) {
	for _, tc := range []struct {
		name, options string
		failfast      bool
	}{
		{"fail fast", "failfast_all_unhealthy_upstreams", true},
		{"random fallback", "", false},
		{"maximum threshold", "max_fails 4294967295\nfailfast_all_unhealthy_upstreams", true},
		{"TCP checks", "force_tcp\nfailfast_all_unhealthy_upstreams", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			primary := newStartupUpstream(t)
			backup := newStartupUpstream(t)
			input := fmt.Sprintf("forward . %s %s {\nstart_failed\npolicy sequential\nhealth_check 10ms no_rec domain probe.example.\n%s\n}", primary.addr, backup.addr, tc.options)
			fs, err := parseForward(caddy.NewTestController("dns", input))
			if err != nil {
				t.Fatal(err)
			}
			f := fs[0]
			if err := f.OnStartup(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = f.OnShutdown() })
			for i, u := range []*startupUpstream{primary, backup} {
				select {
				case q := <-u.probes:
					if q.Question[0].Name != "probe.example." || q.Question[0].Qtype != dns.TypeNS || q.RecursionDesired {
						t.Fatalf("unexpected startup probe: %s", q)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("no startup probe received")
				}
				if !f.proxies[i].Down(f.maxfails) {
					t.Fatal("upstream is healthy before its probe returns")
				}
			}
			query := func() int {
				t.Helper()
				w := dnstest.NewRecorder(&test.ResponseWriter{})
				rcode, _ := f.ServeDNS(t.Context(), w, new(dns.Msg).SetQuestion("example.org.", dns.TypeA))
				return rcode
			}
			wantCode := dns.RcodeSuccess
			if tc.failfast {
				wantCode = dns.RcodeServerFailure
			}
			if got := query(); got != wantCode {
				t.Fatalf("all-down rcode = %d, want %d", got, wantCode)
			}
			beforePrimary, beforeBackup := primary.queries.Load(), backup.queries.Load()
			wantQueries := int32(1)
			if tc.failfast {
				wantQueries = 0
			}
			if beforePrimary+beforeBackup != wantQueries {
				t.Fatal("all-down policy forwarded an unexpected number of queries")
			}

			backup.release()
			waitStartupHealthy(t, func() bool { return !f.proxies[1].Down(f.maxfails) })
			if got := query(); got != dns.RcodeSuccess {
				t.Fatalf("backup rcode = %d", got)
			}
			if primary.queries.Load() != beforePrimary || backup.queries.Load() != beforeBackup+1 {
				t.Fatal("query did not select the healthy backup")
			}

			primary.release()
			waitStartupHealthy(t, func() bool { return !f.proxies[0].Down(f.maxfails) })
			if got := query(); got != dns.RcodeSuccess {
				t.Fatalf("primary rcode = %d", got)
			}
			if primary.queries.Load() != beforePrimary+1 {
				t.Fatal("recovered primary was not selected")
			}
		})
	}
}

func TestStartupHealthDefaultUnchanged(t *testing.T) {
	u := newStartupUpstream(t)
	fs, err := parseForward(caddy.NewTestController("dns", "forward . "+u.addr))
	if err != nil {
		t.Fatal(err)
	}
	f := fs[0]
	if err := f.OnStartup(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.OnShutdown() })
	if f.proxies[0].Down(f.maxfails) {
		t.Fatal("default upstream started unhealthy")
	}
	w := dnstest.NewRecorder(&test.ResponseWriter{})
	if rcode, err := f.ServeDNS(t.Context(), w, new(dns.Msg).SetQuestion("example.org.", dns.TypeA)); err != nil || rcode != dns.RcodeSuccess {
		t.Fatalf("query = %d, %v", rcode, err)
	}
	select {
	case <-u.probes:
		t.Fatal("default startup initiated a probe")
	default:
	}
}

type startupUpstream struct {
	addr    string
	release func()
	probes  chan *dns.Msg
	queries atomic.Int32
}

func newStartupUpstream(t *testing.T) *startupUpstream {
	t.Helper()
	gate := make(chan struct{})
	u := &startupUpstream{probes: make(chan *dns.Msg, 16), release: sync.OnceFunc(func() { close(gate) })}
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg).SetReply(r)
		if r.Question[0].Qtype == dns.TypeNS {
			select {
			case u.probes <- r.Copy():
			default:
			}
			<-gate
			// Even REFUSED proves that the endpoint is reachable.
			m.Rcode = dns.RcodeRefused
		} else {
			u.queries.Add(1)
			m.Answer = []dns.RR{test.A("example.org. 30 IN A 192.0.2.1")}
		}
		_ = w.WriteMsg(m)
	})
	_, port, err := net.SplitHostPort(s.Addr)
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	u.addr = net.JoinHostPort("127.0.0.1", port)
	t.Cleanup(s.Close)
	t.Cleanup(u.release)
	return u
}

func waitStartupHealthy(t *testing.T, healthy func() bool) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !healthy() {
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("health probe did not recover upstream")
		}
	}
}
