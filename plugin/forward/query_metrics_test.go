package forward

import (
	"fmt"
	"net"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

func TestQueryMetricsConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, option string
		wantErr      bool
	}{
		{"disabled", "", false},
		{"selected types", "qtype_for_metrics A AAAA", false},
		{"case insensitive", "qtype_for_metrics a aaaa", false},
		{"repeated type", "qtype_for_metrics A A", false},
		{"generic type", "qtype_for_metrics TYPE65400 type65535", false},
		{"ANY query type", "qtype_for_metrics ANY", false},
		{"missing arguments", "qtype_for_metrics", true},
		{"unknown type", "qtype_for_metrics NOTATYPE", true},
		{"generic missing code", "qtype_for_metrics TYPE", true},
		{"negative code", "qtype_for_metrics TYPE-1", true},
		{"overflow code", "qtype_for_metrics TYPE65536", true},
		{"duplicate option", "qtype_for_metrics A\nqtype_for_metrics AAAA", true},
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

func TestQueryMetricsFailover(t *testing.T) {
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
 qtype_for_metrics A
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
	type series struct{ upstream, rcode, name, qtype string }
	readCounts := func() map[series]uint64 {
		families, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			t.Fatal(err)
		}
		counts := make(map[series]uint64)
		for _, family := range families {
			if family.GetName() != "coredns_proxy_request_duration_seconds" {
				continue
			}
			for _, metric := range family.GetMetric() {
				labels := make(map[string]string)
				for _, label := range metric.GetLabel() {
					labels[label.GetName()] = label.GetValue()
				}
				if labels["proxy_name"] != "forward" || (labels["to"] != upstreams[0] && labels["to"] != upstreams[1]) {
					continue
				}
				wantLabels := 3
				if labels["query_name"] != "" {
					wantLabels = 5
				}
				if len(labels) != wantLabels {
					t.Fatalf("unexpected labels: %v", labels)
				}
				key := series{labels["to"], labels["rcode"], labels["query_name"], labels["query_type"]}
				counts[key] = metric.GetHistogram().GetSampleCount()
			}
		}
		return counts
	}
	// A previous test may have used the same ephemeral upstream port.
	before := readCounts()
	for _, qtype := range []uint16{dns.TypeA, dns.TypeTXT} {
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		if _, err := f.ServeDNS(t.Context(), rec, new(dns.Msg).SetQuestion("leaked.cluster.local.", qtype)); err != nil {
			t.Fatal(err)
		}
		if rec.Msg == nil || rec.Msg.Rcode != dns.RcodeSuccess {
			t.Fatalf("failover response: %v", rec.Msg)
		}
	}
	found := 0
	for key, count := range readCounts() {
		count -= before[key]
		if count == 0 {
			continue
		}
		found++
		if count != 1 {
			t.Fatalf("%v: observation count = %d, want 1", key, count)
		}
		wantRcode := "SERVFAIL"
		if key.upstream == upstreams[1] {
			wantRcode = "NOERROR"
		}
		if key.rcode != wantRcode {
			t.Errorf("RCODE = %q, want %q", key.rcode, wantRcode)
		}
		if (key.name != "" || key.qtype != "") && (key.name != "leaked.cluster.local." || key.qtype != "A") {
			t.Errorf("unexpected query labels: %v", key)
		}
	}
	if found != 4 {
		t.Fatalf("found %d upstream series, want two detailed and two ordinary series", found)
	}
}
