package cache

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

func sharingQuery(do bool) *dns.Msg {
	q := new(dns.Msg)
	q.SetQuestion("probe.example.", dns.TypeA)
	q.SetEdns0(1232, do)
	return q
}

func sharingResponse(q *dns.Msg, kind string, signed bool) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(q)
	m.Authoritative, m.RecursionAvailable, m.AuthenticatedData = true, true, true
	soa := test.SOA("example. 300 IN SOA ns.example. hostmaster.example. 1 60 60 600 300")
	sig := test.RRSIG("probe.example. 300 IN RRSIG A 8 2 300 20300101000000 20200101000000 1 example. AQ==")
	switch kind {
	case "positive":
		m.Answer = []dns.RR{test.A("probe.example. 300 IN A 192.0.2.1")}
		if signed {
			m.Answer = append(m.Answer, sig)
		}
	case "cname":
		m.Answer = []dns.RR{test.CNAME("probe.example. 300 IN CNAME target.example."), test.A("target.example. 300 IN A 192.0.2.2")}
		if signed {
			m.Answer = append(m.Answer, sig)
		}
	case "dname":
		m.Answer = []dns.RR{test.DNAME("example. 300 IN DNAME other."), test.CNAME("probe.example. 300 IN CNAME probe.other."), test.A("probe.other. 300 IN A 192.0.2.3")}
		if signed {
			m.Answer = append(m.Answer, sig)
		}
	case "nxdomain", "nodata", "cname-nodata":
		if kind == "nxdomain" {
			m.Rcode = dns.RcodeNameError
		}
		if kind == "cname-nodata" {
			m.Answer = []dns.RR{test.CNAME("probe.example. 300 IN CNAME empty.example.")}
		}
		m.Ns = []dns.RR{soa}
		if signed {
			m.Ns = append(m.Ns, test.NSEC("probe.example. 300 IN NSEC z.example. A RRSIG NSEC"), sig)
		}
	case "servfail":
		m.Rcode = dns.RcodeServerFailure
	case "delegation":
		m.Ns = []dns.RR{test.NS("probe.example. 300 IN NS ns.probe.example.")}
		if signed {
			m.Ns = append(m.Ns, &dns.DS{Hdr: dns.RR_Header{Name: "probe.example.", Rrtype: dns.TypeDS, Class: dns.ClassINET, Ttl: 300}})
		}
	}
	// Upstream deliberately omits OPT: capability comes from acquisition DO.
	return m
}

func sharingServe(t *testing.T, c *Cache, q *dns.Msg) *dns.Msg {
	t.Helper()
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := c.ServeDNS(context.Background(), rec, q); err != nil {
		t.Fatal(err)
	}
	if rec.Msg == nil {
		t.Fatal("no response")
	}
	return rec.Msg
}

func TestDOSharingResponseKinds(t *testing.T) {
	for _, kind := range []string{"positive", "cname", "dname", "nxdomain", "nodata", "cname-nodata", "servfail", "delegation"} {
		for _, signed := range []bool{false, true} {
			for _, upgrade := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/signed=%t/upgrade=%t", kind, signed, upgrade), func(t *testing.T) {
					c := New()
					c.now = func() time.Time { return time.Unix(1800000000, 0) }
					calls := 0
					c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, q *dns.Msg) (int, error) {
						calls++
						return 0, w.WriteMsg(sharingResponse(q, kind, signed && q.IsEdns0().Do()))
					})
					order := []bool{true, false, true, false}
					want := 1
					if upgrade {
						order = []bool{false, true, false, true}
						want = 2
					}
					for _, do := range order {
						q := sharingQuery(do)
						q.AuthenticatedData = !do
						m := sharingServe(t, c, q)
						ttl := uint32(300)
						if kind == "servfail" || kind == "delegation" {
							ttl = 5
						}
						sharingAssertResponse(t, m, kind, signed && do, ttl)
						sharingAssertRecords(t, "additional", m.Extra, nil)
						if !m.Authoritative || !m.AuthenticatedData || !m.RecursionAvailable || m.CheckingDisabled != q.CheckingDisabled {
							t.Fatalf("flags: %v", m.MsgHdr)
						}
						for _, sec := range [][]dns.RR{m.Answer, m.Ns, m.Extra} {
							for _, rr := range sec {
								if !do && (rr.Header().Rrtype == dns.TypeRRSIG || rr.Header().Rrtype == dns.TypeNSEC || rr.Header().Rrtype == dns.TypeDS) {
									t.Fatalf("DNSSEC leaked: %s", rr)
								}
							}
						}
					}
					if calls != want || c.pcache.Len()+c.ncache.Len() != 1 {
						t.Fatalf("calls=%d entries=%d, want %d/1", calls, c.pcache.Len()+c.ncache.Len(), want)
					}
				})
			}
		}
	}
}

func TestDOExplicitTypesAndImmutableSections(t *testing.T) {
	types := []uint16{dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC, dns.TypeNSEC3, dns.TypeDS, dns.TypeDNSKEY, dns.TypeANY}
	records := []dns.RR{
		test.A("probe.example. 300 IN A 192.0.2.1"),
		test.RRSIG("probe.example. 300 IN RRSIG A 8 2 300 20300101000000 20200101000000 1 example. AQ=="),
		test.NSEC("probe.example. 300 IN NSEC z.example. A RRSIG NSEC"),
		&dns.NSEC3{Hdr: dns.RR_Header{Name: "probe.example.", Rrtype: dns.TypeNSEC3, Class: dns.ClassINET, Ttl: 300}, Hash: 1, HashLength: 20, NextDomain: "00000000000000000000000000000000", TypeBitMap: []uint16{dns.TypeA}},
		&dns.DS{Hdr: dns.RR_Header{Name: "probe.example.", Rrtype: dns.TypeDS, Class: dns.ClassINET, Ttl: 300}},
		&dns.DNSKEY{Hdr: dns.RR_Header{Name: "probe.example.", Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 300}},
	}
	for _, qt := range types {
		t.Run(dns.TypeToString[qt], func(t *testing.T) {
			q := sharingQuery(true)
			q.Question[0].Qtype = qt
			m := new(dns.Msg)
			m.SetReply(q)
			m.Answer = records
			m.Ns = records
			m.Extra = records
			before := m.Copy()
			now := time.Now()
			i := newItem(m, now, 300*time.Second)
			out := i.toMsg(q, now.Add(10*time.Second), false, false)
			explicit := map[uint16]string{
				dns.TypeRRSIG:  "probe.example. 290 IN RRSIG A 8 2 300 20300101000000 20200101000000 1 example. AQ==",
				dns.TypeNSEC:   "probe.example. 290 IN NSEC z.example. A RRSIG NSEC",
				dns.TypeNSEC3:  "probe.example. 290 IN NSEC3 1 0 0 - 00000000000000000000000000000000 A",
				dns.TypeDS:     "probe.example. 290 IN DS 0 0 0",
				dns.TypeDNSKEY: "probe.example. 290 IN DNSKEY 0 0 0",
			}
			want := []string{"probe.example. 290 IN A 192.0.2.1"}
			if qt == dns.TypeANY {
				for _, typ := range []uint16{dns.TypeRRSIG, dns.TypeNSEC, dns.TypeNSEC3, dns.TypeDS, dns.TypeDNSKEY} {
					want = append(want, explicit[typ])
				}
			} else if qt != dns.TypeA {
				want = append(want, explicit[qt])
			}
			sharingAssertRecords(t, "explicit answer", out.Answer, want)
			sharingAssertRecords(t, "authority", out.Ns, []string{"probe.example. 290 IN A 192.0.2.1"})
			sharingAssertRecords(t, "additional", out.Extra, []string{"probe.example. 290 IN A 192.0.2.1"})
			out.Answer[0].Header().Ttl = 1
			if !reflect.DeepEqual(m, before) {
				t.Fatal("shared records changed")
			}
			signed := i.toMsg(q, now, true, true)
			if len(signed.Answer) != len(records) || signed.Answer[0].Header().Ttl != 300 {
				t.Fatal("signed data destroyed")
			}
		})
	}
}

func TestDONoAlwaysDOAndQuestionIsolation(t *testing.T) {
	c := New()
	calls := 0
	c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, q *dns.Msg) (int, error) {
		calls++
		if q.IsEdns0() != nil {
			t.Error("unexpected EDNS on plain miss")
		}
		m := sharingResponse(q, "positive", false)
		m.Answer[0].Header().Class = q.Question[0].Qclass
		return 0, w.WriteMsg(m)
	})
	q := sharingQuery(false)
	q.Extra = nil
	sharingServe(t, c, q)
	sharingServe(t, c, q)
	q.Question[0].Qclass = dns.ClassCHAOS
	sharingServe(t, c, q)
	sharingServe(t, c, q)
	if calls != 2 {
		t.Fatalf("class isolation: calls=%d", calls)
	}
}

func TestDOLateUnsignedCannotReplaceFresh(t *testing.T) {
	for _, older := range []string{"positive", "nxdomain", "servfail"} {
		for _, newer := range []string{"positive", "nxdomain", "empty"} {
			t.Run(older+"/"+newer, func(t *testing.T) {
				c := New()
				c.preferPositive = newer == "empty"
				c.now = func() time.Time { return time.Unix(1800000000, 0) }
				started := make(chan struct{})
				release := make(chan struct{})
				done := make(chan struct{})
				var calls atomic.Int32
				c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, q *dns.Msg) (int, error) {
					calls.Add(1)
					if !q.IsEdns0().Do() {
						close(started)
						<-release
						return 0, w.WriteMsg(sharingResponse(q, older, false))
					}
					return 0, w.WriteMsg(sharingResponse(q, newer, true))
				})
				go func() {
					defer close(done)
					rec := dnstest.NewRecorder(&test.ResponseWriter{})
					c.ServeDNS(context.Background(), rec, sharingQuery(false))
				}()
				<-started
				sharingServe(t, c, sharingQuery(true))
				close(release)
				<-done
				for _, do := range []bool{false, true} {
					got := sharingServe(t, c, sharingQuery(do))
					sharingAssertResponse(t, got, newer, do, 300)
					want := dns.RcodeSuccess
					if newer == "nxdomain" {
						want = dns.RcodeNameError
					}
					if got.Rcode != want {
						t.Fatalf("late response won: %s", got)
					}
				}
				if calls.Load() != 2 || c.pcache.Len()+c.ncache.Len() != 1 {
					t.Fatalf("calls=%d entries=%d", calls.Load(), c.pcache.Len()+c.ncache.Len())
				}
			})
		}
	}
}

func TestDOUpgradeAcrossResponseKinds(t *testing.T) {
	for _, first := range []string{"positive", "nxdomain"} {
		t.Run(first, func(t *testing.T) {
			c := New()
			c.now = func() time.Time { return time.Unix(1800000000, 0) }
			calls := 0
			second := "positive"
			if first == "positive" {
				second = "nxdomain"
			}
			c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, q *dns.Msg) (int, error) {
				calls++
				kind := first
				if q.IsEdns0().Do() {
					kind = second
				}
				return 0, w.WriteMsg(sharingResponse(q, kind, q.IsEdns0().Do()))
			})
			sharingServe(t, c, sharingQuery(false))
			fresh := sharingServe(t, c, sharingQuery(true))
			hit := sharingServe(t, c, sharingQuery(false))
			sharingAssertResponse(t, fresh, second, true, 300)
			sharingAssertResponse(t, hit, second, false, 300)
			if fresh.Rcode != hit.Rcode || calls != 2 || c.pcache.Len()+c.ncache.Len() != 1 {
				t.Fatalf("upgrade shadowed: codes %d/%d calls %d entries %d", fresh.Rcode, hit.Rcode, calls, c.pcache.Len()+c.ncache.Len())
			}
		})
	}
}

func TestDORefreshCapability(t *testing.T) {
	for _, mode := range []string{"prefetch", "stale-immediate", "verify", "verify-fast", "verify-detached"} {
		for _, edns := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/edns=%t", mode, edns), func(t *testing.T) {
				c := New()
				base := time.Now()
				var seconds atomic.Int64
				c.now = func() time.Time { return base.Add(time.Duration(seconds.Load()) * time.Second) }
				c.staleUpTo = time.Hour
				if mode == "prefetch" {
					c.prefetch = 1
					c.percentage = 10
				}
				if mode == "verify" || mode == "verify-fast" || mode == "verify-detached" {
					c.verifyStale = true
				}
				if mode == "verify-fast" {
					c.verifyStaleTimeout = time.Second
				}
				if mode == "verify-detached" {
					c.verifyStaleTimeout = 10 * time.Millisecond
				}
				done := make(chan struct{})
				release := make(chan struct{})
				var calls atomic.Int32
				c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, q *dns.Msg) (int, error) {
					n := calls.Add(1)
					if n == 2 {
						defer close(done)
						opt := q.IsEdns0()
						if opt == nil || !opt.Do() {
							t.Error("refresh lost DO")
						}
						if edns && opt != nil && opt.UDPSize() != 1232 {
							t.Error("refresh changed client's EDNS size")
						}
						if mode == "verify-detached" {
							<-release
						}
					}
					m := sharingResponse(q, "positive", true)
					if n == 2 {
						m.Answer = []dns.RR{test.A("probe.example. 300 IN A 192.0.2.2"), test.RRSIG("probe.example. 300 IN RRSIG A 8 2 300 20300101000000 20200101000000 2 example. Ag==")}
					}
					return 0, w.WriteMsg(m)
				})
				sharingServe(t, c, sharingQuery(true))
				key := hash("probe.example.", dns.TypeA, dns.ClassINET, false)
				old, _ := c.pcache.Get(key)
				seconds.Store(301)
				if mode == "prefetch" {
					seconds.Store(275)
				}
				q := sharingQuery(false)
				if !edns {
					q.Extra = nil
				}
				before := q.Copy()
				got := sharingServe(t, c, q)
				if len(got.Answer) != 1 || got.AuthenticatedData {
					t.Fatal("unsigned refresh response not filtered")
				}
				wantAnswer := "probe.example. 0 IN A 192.0.2.1"
				switch mode {
				case "prefetch":
					wantAnswer = "probe.example. 25 IN A 192.0.2.1"
				case "verify", "verify-fast":
					wantAnswer = "probe.example. 300 IN A 192.0.2.2"
				}
				sharingAssertRecords(t, "refresh answer", got.Answer, []string{wantAnswer})
				if mode == "verify-detached" {
					close(release)
				}
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Fatal("no refresh")
				}
				// Wait for frequency transfer/backoff publication as well as WriteMsg.
				deadline := time.Now().Add(time.Second)
				for old.refreshing.Load() && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if old.refreshing.Load() {
					t.Fatal("refresh not finished")
				}
				fresh, _ := c.pcache.Get(key)
				if fresh == old || !fresh.do {
					t.Fatal("not upgraded/refreshed")
				}
				// Msg.Copy normalizes nil RR slices to empty slices.
				if !reflect.DeepEqual(q.Copy(), before) {
					t.Fatal("client request mutated")
				}
				if got.IsEdns0() != nil {
					t.Fatal("cache leaked upstream OPT")
				}
				signed := sharingServe(t, c, sharingQuery(true))
				sharingAssertRecords(t, "refreshed signed answer", signed.Answer, []string{
					"probe.example. 300 IN A 192.0.2.2",
					"probe.example. 300 IN RRSIG A 8 2 300 20300101000000 20200101000000 2 example. Ag==",
				})
				sharingAssertRecords(t, "refreshed authority", signed.Ns, nil)
				sharingAssertRecords(t, "refreshed additional", signed.Extra, nil)
				if signed.Rcode != dns.RcodeSuccess || !signed.Authoritative || !signed.RecursionAvailable || !signed.AuthenticatedData {
					t.Fatalf("refreshed signed flags: %v", signed.MsgHdr)
				}
				if calls.Load() != 2 {
					t.Fatalf("signed hit missed: %d", calls.Load())
				}
			})
		}
	}
}

// Use the same hop-by-hop EDNS normalization as the server response writer.
type sharingWireWriter struct {
	dns.ResponseWriter
	state request.Request
}

func (w *sharingWireWriter) WriteMsg(m *dns.Msg) error {
	w.state.SizeAndDo(m)
	return w.ResponseWriter.WriteMsg(m)
}

func TestDOSharingWire(t *testing.T) {
	for _, network := range []string{"udp", "tcp"} {
		t.Run(network, func(t *testing.T) {
			for _, kind := range []string{"positive", "cname", "dname", "nxdomain", "nodata", "cname-nodata", "servfail", "delegation"} {
				t.Run(kind, func(t *testing.T) {
					c := New()
					var seconds atomic.Int64
					c.now = func() time.Time { return time.Unix(1800000000+seconds.Load(), 0) }
					var calls atomic.Int32
					c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, q *dns.Msg) (int, error) {
						calls.Add(1)
						m := sharingResponse(q, kind, q.IsEdns0() != nil && q.IsEdns0().Do())
						m.Extra = []dns.RR{test.A("ns.example. 300 IN A 192.0.2.53"), test.DNSKEY("example. 300 IN DNSKEY 256 3 8 AQ==")}
						return 0, w.WriteMsg(m)
					})
					server := &dns.Server{Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) {
						cw := &sharingWireWriter{ResponseWriter: w, state: request.Request{Req: q, W: w}}
						c.ServeDNS(context.Background(), cw, q)
					})}
					var addr string
					if network == "udp" {
						pc, err := net.ListenPacket("udp", "127.0.0.1:0")
						if err != nil {
							t.Fatal(err)
						}
						server.PacketConn = pc
						addr = pc.LocalAddr().String()
					} else {
						ln, err := net.Listen("tcp", "127.0.0.1:0")
						if err != nil {
							t.Fatal(err)
						}
						server.Listener = ln
						addr = ln.Addr().String()
					}
					ready := make(chan struct{})
					server.NotifyStartedFunc = func() { close(ready) }
					done := make(chan error, 1)
					go func() { done <- server.ActivateAndServe() }()
					<-ready
					t.Cleanup(func() {
						if err := server.Shutdown(); err != nil {
							t.Error(err)
						}
						if err := <-done; err != nil {
							t.Error(err)
						}
					})
					client := &dns.Client{Net: network, Timeout: time.Second}
					for step, do := range []bool{true, false, true, false} {
						seconds.Store(int64(step))
						q := sharingQuery(do)
						q.CheckingDisabled = true
						got, _, err := client.Exchange(q, addr)
						if err != nil {
							t.Fatal(err)
						}
						ttl := uint32(300 - step)
						if kind == "servfail" || kind == "delegation" {
							ttl = uint32(5 - step)
						}
						sharingAssertResponse(t, got, kind, do, ttl)
						opt := got.IsEdns0()
						if !got.Response || got.Opcode != dns.OpcodeQuery || got.Id != q.Id || got.Truncated || !got.RecursionDesired || !got.RecursionAvailable || !got.CheckingDisabled || !got.Authoritative || got.AuthenticatedData != do || len(got.Question) != 1 || got.Question[0] != q.Question[0] || opt == nil || opt.Do() != do || opt.UDPSize() != 1232 || opt.Version() != 0 || len(opt.Option) != 0 {
							t.Fatalf("wire response %s", got)
						}
						wantExtra := []string{fmt.Sprintf("ns.example. %d IN A 192.0.2.53", ttl)}
						if do {
							wantExtra = append(wantExtra, fmt.Sprintf("example. %d IN DNSKEY 256 3 8 AQ==", ttl))
						}
						if len(got.Extra) != len(wantExtra)+1 || got.Extra[len(got.Extra)-1] != opt {
							t.Fatalf("additional/OPT layout: %v", got.Extra)
						}
						sharingAssertRecords(t, "wire additional", got.Extra[:len(got.Extra)-1], wantExtra)
						if calls.Load() != 1 {
							t.Fatalf("step %d backend calls=%d want1", step, calls.Load())
						}
					}
					if calls.Load() != 1 {
						t.Fatalf("wire did not share: %d", calls.Load())
					}
				})
			}
		})
	}
}

func BenchmarkDOSharingHotHit(b *testing.B) {
	for _, do := range []bool{false, true} {
		b.Run(fmt.Sprintf("do=%t", do), func(b *testing.B) {
			c := New()
			c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, q *dns.Msg) (int, error) {
				return 0, w.WriteMsg(sharingResponse(q, "positive", true))
			})
			w := dnstest.NewRecorder(&test.ResponseWriter{})
			c.ServeDNS(context.Background(), w, sharingQuery(true))
			q := sharingQuery(do)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				c.ServeDNS(context.Background(), w, q)
			}
		})
	}
}
