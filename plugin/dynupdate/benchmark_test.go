package dynupdate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	coretest "github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// BenchmarkUpdate measures a bounded, changing zone rather than duplicate
// no-op updates. Durable mode uses bbolt's normal synchronous commit path.
func BenchmarkUpdate(b *testing.B) {
	for _, count := range []int{100, 1000, defaultMaxRecords} {
		for _, durable := range []bool{false, true} {
			for _, readers := range []int{0, 4} {
				b.Run(fmt.Sprintf("records=%d/durable=%t/readers=%d", count, durable, readers), func(b *testing.B) {
					d := benchmarkZone(b, count, durable)
					updates := make([][]dns.RR, 2)
					for i := range updates {
						rr, err := dns.NewRR(fmt.Sprintf("change.example.org. 60 IN A 192.0.2.%d", i+1))
						if err != nil {
							b.Fatal(err)
						}
						m := new(dns.Msg).SetUpdate(testZone)
						m.RemoveRRset([]dns.RR{rr})
						m.Insert([]dns.RR{rr})
						updates[i] = m.Ns
					}
					var queries, queryNanos, maxQueryNanos atomic.Int64
					var wg sync.WaitGroup
					startQueries := make(chan struct{})
					stop := make(chan struct{})
					errors := make(chan error, readers)
					for range readers {
						wg.Go(func() {
							query := new(dns.Msg).SetQuestion("change.example.org.", dns.TypeA)
							<-startQueries
							for {
								select {
								case <-stop:
									return
								default:
								}
								start := time.Now()
								w := dnstest.NewRecorder(&coretest.ResponseWriter{})
								code, err := d.ServeDNS(context.Background(), w, query)
								if err != nil || code != dns.RcodeSuccess || w.Msg == nil || len(w.Msg.Answer) != 1 {
									errors <- fmt.Errorf("concurrent query: code=%d err=%v reply=%v", code, err, w.Msg)
									return
								}
								latency := time.Since(start).Nanoseconds()
								queries.Add(1)
								queryNanos.Add(latency)
								for old := maxQueryNanos.Load(); latency > old; old = maxQueryNanos.Load() {
									if maxQueryNanos.CompareAndSwap(old, latency) {
										break
									}
								}
							}
						})
					}
					b.ReportAllocs()
					b.ResetTimer()
					start := time.Now()
					close(startQueries)
					for i := range b.N {
						code, err := d.applyUpdate(testKey, nil, updates[i%2])
						if code != dns.RcodeSuccess || err != nil {
							b.Errorf("update: code=%d err=%v", code, err)
							break
						}
					}
					b.StopTimer()
					close(stop)
					wg.Wait()
					elapsed := time.Since(start)
					close(errors)
					for err := range errors {
						b.Error(err)
					}
					if n := queries.Load(); n > 0 {
						b.ReportMetric(float64(queryNanos.Load())/float64(n), "ns/query")
						b.ReportMetric(float64(maxQueryNanos.Load()), "max-ns/query")
						b.ReportMetric(float64(n)/elapsed.Seconds(), "queries/s")
					}
				})
			}
		}
	}
}

func benchmarkZone(b *testing.B, count int, durable bool) *DynUpdate {
	b.Helper()
	texts := []string{
		"example.org. 60 IN SOA ns.example.org. hostmaster.example.org. 10 60 60 60 60",
		"example.org. 60 IN NS ns.example.org.",
		"change.example.org. 60 IN A 192.0.2.254",
	}
	for i := len(texts); i < count; i++ {
		texts = append(texts, fmt.Sprintf("host-%d.example.org. 60 IN A 192.0.2.100", i))
	}
	d := &DynUpdate{Zone: testZone, permissions: []permission{{key: testKey, name: allNames, allTypes: true}}}
	for _, text := range texts {
		rr, err := dns.NewRR(text)
		if err != nil {
			b.Fatal(err)
		}
		d.records = append(d.records, rr)
	}
	var err error
	d.view, err = d.build(d.records)
	if err != nil {
		b.Fatal(err)
	}
	if durable {
		dir := b.TempDir()
		d.seed, d.database = filepath.Join(dir, "seed.zone"), filepath.Join(dir, "zone.db")
		if err := os.WriteFile(d.seed, []byte(strings.Join(texts, "\n")+"\n"), 0600); err != nil {
			b.Fatal(err)
		}
		if err := d.ensureStore(); err != nil {
			b.Fatal(err)
		}
	}
	b.Cleanup(func() {
		if err := d.close(); err != nil {
			b.Error(err)
		}
	})
	return d
}
