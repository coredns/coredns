package forward

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

func TestQueryExemplarsConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, option string
		wantErr      bool
	}{
		{"disabled", "", false},
		{"all types", "query_exemplars cluster.local 100", false},
		{"selected types", "query_exemplars CLUSTER.local. 1 A AAAA", false},
		{"all names", "query_exemplars . 10 a aaaa", false},
		{"missing arguments", "query_exemplars", true},
		{"missing interval", "query_exemplars cluster.local", true},
		{"zero interval", "query_exemplars cluster.local 0", true},
		{"negative interval", "query_exemplars cluster.local -1", true},
		{"invalid interval", "query_exemplars cluster.local often", true},
		{"overflow interval", "query_exemplars cluster.local 18446744073709551616", true},
		{"invalid name", "query_exemplars bad..name 1", true},
		{"long label", "query_exemplars " + strings.Repeat("a", 64) + ". 1", true},
		{"unknown type", "query_exemplars cluster.local 1 NOTATYPE", true},
		{"duplicate option", "query_exemplars . 1\nquery_exemplars cluster.local 1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", "forward . 127.0.0.1 {\n"+tc.option+"\n}")
			_, err := parseForward(c)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseForward() error = %v, want error %v", err, tc.wantErr)
			}
		})
	}
}

func TestQueryExemplarsFailover(t *testing.T) {
	upstreams := make([]string, 0, 2)
	for _, rcode := range []int{dns.RcodeServerFailure, dns.RcodeSuccess} {
		srv := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
			_ = w.WriteMsg(new(dns.Msg).SetRcode(r, rcode))
		})
		t.Cleanup(srv.Close)
		_, port, err := net.SplitHostPort(srv.Addr)
		if err != nil {
			t.Fatal(err)
		}
		upstreams = append(upstreams, net.JoinHostPort("127.0.0.1", port))
	}
	c := caddy.NewTestController("dns", fmt.Sprintf(`forward . %s %s {
 policy sequential
 failover SERVFAIL
 query_exemplars cluster.local 2 A
}`, upstreams[0], upstreams[1]))
	fs, err := parseForward(c)
	if err != nil {
		t.Fatal(err)
	}
	f := fs[0]
	if err := f.OnStartup(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.OnShutdown() })
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := f.ServeDNS(t.Context(), rec, new(dns.Msg).SetQuestion("leaked.cluster.local.", dns.TypeA)); err != nil {
		t.Fatal(err)
	}
	if rec.Msg == nil || rec.Msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("failover response: %v", rec.Msg)
	}
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, family := range families {
		if family.GetName() != "coredns_proxy_request_duration_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string)
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["proxy_name"] != "forward" {
				continue
			}
			index := -1
			for i, upstream := range upstreams {
				if labels["to"] == upstream {
					index = i
				}
			}
			if index == -1 {
				continue
			}
			found++
			if metric.GetHistogram().GetSampleCount() != 1 {
				t.Fatalf("observation count: %v", metric)
			}
			wantRcode := "SERVFAIL"
			if index == 1 {
				wantRcode = "NOERROR"
			}
			if labels["rcode"] != wantRcode {
				t.Errorf("upstream %d RCODE = %q, want %q", index, labels["rcode"], wantRcode)
			}
			hasExemplar := false
			for _, bucket := range metric.GetHistogram().GetBucket() {
				if bucket.GetExemplar() != nil {
					hasExemplar = true
				}
			}
			if hasExemplar != (index == 1) {
				t.Errorf("upstream %d exemplar present = %v; only the second response should be sampled", index, hasExemplar)
			}
		}
	}
	if found != 2 {
		t.Fatalf("found %d upstream series, want 2", found)
	}
}
