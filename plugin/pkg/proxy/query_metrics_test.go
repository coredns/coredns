package proxy

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestQueryMetricsObservations(t *testing.T) {
	m := newRequestDurationMetrics()
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(m)
	p := &Proxy{proxyName: "forward", addr: "127.0.0.1:53"}
	observe := func(name string, qtype uint16) {
		m.observe(p, request.Request{Req: new(dns.Msg).SetQuestion(name, qtype)}, "NOERROR", 0.25)
	}
	observe("disabled.example.", dns.TypeA)
	types := []uint16{dns.TypeA, dns.TypeAAAA, 65400}
	p.SetQueryTypesForMetrics(types)
	types[0] = dns.TypeTXT // Configuration does not retain the caller's slice.
	observe("API.Example.", dns.TypeA)
	observe("api.example.", dns.TypeA)
	observe("api.example.", dns.TypeAAAA)
	observe("filtered.example.", dns.TypeTXT)
	observe(".", 65400)
	longName := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61) + "."
	observe(longName, dns.TypeA)
	observe("bad\xff.example.", dns.TypeA)
	for _, req := range []*dns.Msg{nil, {}} {
		m.observe(p, request.Request{Req: req}, "NOERROR", 0.25)
	}
	p.SetQueryTypesForMetrics(nil)
	observe("api.example.", dns.TypeA)

	want := map[string]uint64{
		"|":                  5,
		"api.example.|A":     2,
		"api.example.|AAAA":  1,
		".|TYPE65400":        1,
		longName + "|A":      1,
		`bad\255.example.|A`: 1,
	}
	metrics := queryMetricsGather(t, reg)
	if len(metrics) != len(want) {
		t.Fatalf("series = %d, want %d", len(metrics), len(want))
	}
	for _, metric := range metrics {
		labels := queryMetricLabels(metric)
		key := labels["query_name"] + "|" + labels["query_type"]
		count, ok := want[key]
		if !ok {
			t.Fatalf("unexpected series: %v", labels)
		}
		wantLabels := 5
		if key == "|" {
			wantLabels = 3
		}
		if len(labels) != wantLabels || labels["proxy_name"] != "forward" || labels["to"] != p.addr || labels["rcode"] != "NOERROR" {
			t.Errorf("unexpected labels: %v", labels)
		}
		h := metric.GetHistogram()
		if h.GetSampleCount() != count || math.Abs(h.GetSampleSum()-0.25*float64(count)) > 1e-9 {
			t.Errorf("%s: count/sum = %d/%g, want %d/%g", key, h.GetSampleCount(), h.GetSampleSum(), count, 0.25*float64(count))
		}
		if h.Schema == nil {
			t.Errorf("%s: native histogram missing", key)
		}
		for _, bucket := range h.GetBucket() {
			var wantCount uint64
			if bucket.GetUpperBound() >= 0.25 {
				wantCount = count
			}
			if bucket.GetCumulativeCount() != wantCount {
				t.Errorf("%s: bucket %g count = %d, want %d", key, bucket.GetUpperBound(), bucket.GetCumulativeCount(), wantCount)
			}
		}
	}
}

func TestQueryMetricsConcurrentDistinctNames(t *testing.T) {
	m := newRequestDurationMetrics()
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(m)
	p := &Proxy{proxyName: "forward", addr: "127.0.0.1:53"}
	p.SetQueryTypesForMetrics([]uint16{dns.TypeA})
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Go(func() {
			for n := range 125 {
				name := fmt.Sprintf("query-%d.example.", worker*125+n)
				state := request.Request{Req: new(dns.Msg).SetQuestion(name, dns.TypeA)}
				m.observe(p, state, "NOERROR", 0.25)
				m.observe(p, state, "NOERROR", 0.25)
				m.observe(p, request.Request{Req: new(dns.Msg).SetQuestion(name, dns.TypeAAAA)}, "NOERROR", 0.25)
			}
		})
	}
	for range 20 {
		if _, err := reg.Gather(); err != nil {
			t.Error(err)
		}
	}
	wg.Wait()
	metrics := queryMetricsGather(t, reg)
	if len(metrics) != 1001 {
		t.Fatalf("series = %d, want 1000 named series and one ordinary series", len(metrics))
	}
	var total uint64
	for _, metric := range metrics {
		labels := queryMetricLabels(metric)
		count := metric.GetHistogram().GetSampleCount()
		if labels["query_name"] == "" {
			if count != 1000 || len(labels) != 3 {
				t.Errorf("filtered series: %v", metric)
			}
		} else if count != 2 || labels["query_type"] != "A" || len(labels) != 5 {
			t.Errorf("query series: %v", metric)
		}
		total += count
	}
	if total != 3000 {
		t.Fatalf("observations = %d, want 3000", total)
	}
}

func queryMetricLabels(metric *dto.Metric) map[string]string {
	labels := make(map[string]string)
	for _, pair := range metric.GetLabel() {
		labels[pair.GetName()] = pair.GetValue()
	}
	return labels
}

func queryMetricsGather(t *testing.T, reg *prometheus.Registry) []*dto.Metric {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 1 || families[0].GetName() != "coredns_proxy_request_duration_seconds" {
		t.Fatalf("unexpected metric families: %v", families)
	}
	return families[0].GetMetric()
}
