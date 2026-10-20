package proxy

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestQueryExemplarsSelection(t *testing.T) {
	q, err := NewQueryExemplars("CLUSTER.local", 2, []uint16{dns.TypeA, dns.TypeAAAA})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		typeCode uint16
		want     prometheus.Labels
	}{
		{"public.example.", dns.TypeA, nil},
		{"notcluster.local.", dns.TypeA, nil},
		{"cluster.local.example.", dns.TypeA, nil},
		{`escaped\.cluster.local.`, dns.TypeA, nil},
		{"service.cluster.local.", dns.TypeTXT, nil},
		{"first.cluster.local.", dns.TypeA, nil},
		{"SERVICE.cluster.LOCAL.", dns.TypeAAAA, prometheus.Labels{"query_name": "service.cluster.local.", "query_type": "AAAA"}},
		{"third.cluster.local.", dns.TypeA, nil},
		{"cluster.local.", dns.TypeA, prometheus.Labels{"query_name": "cluster.local.", "query_type": "A"}},
	} {
		state := request.Request{Req: new(dns.Msg).SetQuestion(tc.name, tc.typeCode)}
		if got := q.labels(state); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s %s: got %v, want %v", tc.name, dns.Type(tc.typeCode), got, tc.want)
		}
	}
}

func TestQueryExemplarsLimits(t *testing.T) {
	if labels := new(QueryExemplars).labels(request.Request{Req: new(dns.Msg).SetQuestion("example.", dns.TypeA)}); labels != nil {
		t.Fatalf("zero-valued selector returned labels: %v", labels)
	}
	// The label names consume 20 runes, leaving 107 for the QNAME with type A.
	atLimit := strings.Repeat("a", 63) + "." + strings.Repeat("b", 42) + "."
	for _, tc := range []struct {
		name     string
		question string
		typeCode uint16
		want     bool
	}{
		{"root", ".", dns.TypeA, true},
		{"private type", "example.", 65400, true},
		{"at limit", atLimit, dns.TypeA, true},
		{"over limit", strings.Repeat("a", 63) + "." + strings.Repeat("b", 43) + ".", dns.TypeA, false},
		{"longer type", atLimit, dns.TypeAAAA, false},
		{"escaped binary label", "bad\xff.example.", dns.TypeA, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, err := NewQueryExemplars(".", 1, nil)
			if err != nil {
				t.Fatal(err)
			}
			state := request.Request{Req: new(dns.Msg).SetQuestion(tc.question, tc.typeCode)}
			got := q.labels(state)
			if (got != nil) != tc.want {
				t.Fatalf("labels = %v, want exemplar %v", got, tc.want)
			}
			if tc.want && got["query_type"] != dns.Type(tc.typeCode).String() {
				t.Fatalf("query type = %q", got["query_type"])
			}
		})
	}
	q, err := NewQueryExemplars(".", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range []*dns.Msg{nil, {}} {
		if got := q.labels(request.Request{Req: msg}); got != nil {
			t.Fatalf("malformed request produced exemplar %v", got)
		}
	}
	for _, tc := range []struct {
		zone  string
		every uint64
	}{{"", 1}, {"bad..name", 1}, {".", 0}} {
		if _, err := NewQueryExemplars(tc.zone, tc.every, nil); err == nil {
			t.Errorf("accepted invalid configuration %+v", tc)
		}
	}
}

func TestQueryExemplarsConcurrentSampling(t *testing.T) {
	q, err := NewQueryExemplars("cluster.local.", 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	var sampled atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 125 {
				state := request.Request{Req: new(dns.Msg).SetQuestion("service.cluster.local.", dns.TypeA)}
				if q.labels(state) != nil {
					sampled.Add(1)
				}
			}
		})
	}
	wg.Wait()
	if got := sampled.Load(); got != 100 {
		t.Fatalf("sampled %d of 1000 responses, want 100", got)
	}
}

func TestQueryExemplarsCardinality(t *testing.T) {
	q, err := NewQueryExemplars("cluster.local.", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := &Proxy{proxyName: t.Name(), addr: "127.0.0.1:53", queryExemplars: q}
	t.Cleanup(func() { requestDuration.DeleteLabelValues(p.proxyName, p.addr, "NOERROR") })
	const count = 1000
	for i := range count {
		state := request.Request{Req: new(dns.Msg).SetQuestion(fmt.Sprintf("service%d.cluster.local.", i), dns.TypeA)}
		p.observeRequestDuration(state, "NOERROR", 0.001)
	}
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(requestDuration)
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var found int
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			labels := exemplarLabelMap(metric.GetLabel())
			if labels["proxy_name"] != p.proxyName {
				continue
			}
			found++
			if len(labels) != 3 {
				t.Fatalf("series labels changed: %v", labels)
			}
			h := metric.GetHistogram()
			if h.GetSampleCount() != count {
				t.Errorf("histogram count = %d, want %d", h.GetSampleCount(), count)
			}
			var classicExemplars int
			for _, bucket := range h.GetBucket() {
				if bucket.GetExemplar() == nil {
					continue
				}
				classicExemplars++
				if got := exemplarLabelMap(bucket.GetExemplar().GetLabel())["query_name"]; got != "service999.cluster.local." {
					t.Errorf("last bucket exemplar name = %q", got)
				}
			}
			if classicExemplars != 1 {
				t.Errorf("classic exemplars = %d, want 1 for one occupied bucket", classicExemplars)
			}
			if n := len(h.GetExemplars()); n == 0 || n > 10 {
				t.Errorf("native exemplars = %d, want between 1 and 10", n)
			}
		}
	}
	if found != 1 {
		t.Fatalf("histogram series = %d, want 1 after %d distinct names", found, count)
	}
}

func TestQueryExemplarsKeepOrdinaryObservations(t *testing.T) {
	q, err := NewQueryExemplars("cluster.local.", 1, []uint16{dns.TypeA})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, qname string
		qtype       uint16
		enabled     bool
	}{
		{"disabled", "service.cluster.local.", dns.TypeA, false},
		{"excluded name", "public.example.", dns.TypeA, true},
		{"excluded type", "service.cluster.local.", dns.TypeTXT, true},
		{"oversized name", strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + ".cluster.local.", dns.TypeA, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &Proxy{proxyName: t.Name(), addr: "127.0.0.1:53"}
			if tc.enabled {
				p.SetQueryExemplars(q)
			}
			t.Cleanup(func() { requestDuration.DeleteLabelValues(p.proxyName, p.addr, "NXDOMAIN") })
			state := request.Request{Req: new(dns.Msg).SetQuestion(tc.qname, tc.qtype)}
			p.observeRequestDuration(state, "NXDOMAIN", 0.001)
			metric := new(dto.Metric)
			if err := requestDuration.WithLabelValues(p.proxyName, p.addr, "NXDOMAIN").(prometheus.Metric).Write(metric); err != nil {
				t.Fatal(err)
			}
			h := metric.GetHistogram()
			if h.GetSampleCount() != 1 {
				t.Fatalf("observation lost: %v", h)
			}
			for _, b := range h.GetBucket() {
				if b.GetExemplar() != nil {
					t.Fatalf("unexpected exemplar: %v", b.GetExemplar())
				}
			}
			if len(h.GetExemplars()) != 0 {
				t.Fatalf("unexpected native exemplar: %v", h.GetExemplars())
			}
		})
	}
}

func exemplarLabelMap(pairs []*dto.LabelPair) map[string]string {
	labels := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		labels[pair.GetName()] = pair.GetValue()
	}
	return labels
}

func BenchmarkQueryExemplars(b *testing.B) {
	for _, every := range []uint64{0, 1, 1000} {
		b.Run(fmt.Sprintf("every=%d", every), func(b *testing.B) {
			p := &Proxy{proxyName: b.Name(), addr: "127.0.0.1:53"}
			if every != 0 {
				q, err := NewQueryExemplars("cluster.local", every, nil)
				if err != nil {
					b.Fatal(err)
				}
				p.SetQueryExemplars(q)
			}
			b.Cleanup(func() { requestDuration.DeleteLabelValues(p.proxyName, p.addr, "NOERROR") })
			state := request.Request{Req: new(dns.Msg).SetQuestion("service.cluster.local.", dns.TypeA)}
			b.ReportAllocs()
			for b.Loop() {
				p.observeRequestDuration(state, "NOERROR", 0.001)
			}
		})
	}
}
