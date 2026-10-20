package test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/dnstest"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

func TestForwardQueryExemplars(t *testing.T) {
	// Serve the proxy's process-wide metrics without adding request-size samples
	// to the metrics plugin's counters, which other integration tests inspect.
	collector := metrics.New("127.0.0.1:0")
	previousAddr := metrics.ListenAddr
	if err := collector.OnStartup(); err != nil {
		t.Fatal(err)
	}
	metricsURL := "http://" + metrics.ListenAddr + "/metrics"
	metrics.ListenAddr = previousAddr
	t.Cleanup(func() { _ = collector.OnFinalShutdown() })
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%v", enabled), func(t *testing.T) {
			upstream := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
				_ = w.WriteMsg(new(dns.Msg).SetReply(r))
			})
			t.Cleanup(upstream.Close)
			_, port, err := net.SplitHostPort(upstream.Addr)
			if err != nil {
				t.Fatal(err)
			}
			upstreamAddr := net.JoinHostPort("127.0.0.1", port)
			option := ""
			if enabled {
				option = "query_exemplars CLUSTER.local 1 A AAAA"
			}
			corefile := fmt.Sprintf(`.:0 {
	bind 127.0.0.1
	forward . %s {
		%s
	}
}`, upstreamAddr, option)
			inst, udp, tcp, err := CoreDNSServerAndPorts(corefile)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = inst.Stop()
				inst.ShutdownCallbacks()
			})
			cases := []struct {
				proto, name string
				qtype       uint16
				wantSample  bool
			}{
				{"udp", "A.svc.CLUSTER.local.", dns.TypeA, true},
				{"udp", "aaaa.svc.cluster.local.", dns.TypeAAAA, true},
				{"tcp", "tcp-a.svc.cluster.local.", dns.TypeA, true},
				{"tcp", "tcp-aaaa.svc.cluster.local.", dns.TypeAAAA, true},
				{"udp", "public.example.", dns.TypeA, false},
				{"tcp", "notcluster.local.", dns.TypeA, false},
				{"udp", "txt.svc.cluster.local.", dns.TypeTXT, false},
				{"tcp", strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + ".cluster.local.", dns.TypeA, false},
			}
			for _, tc := range cases {
				addr := udp
				if tc.proto == "tcp" {
					addr = tcp
				}
				client := &dns.Client{Net: tc.proto, Timeout: 2 * time.Second}
				reply, _, err := client.Exchange(new(dns.Msg).SetQuestion(tc.name, tc.qtype), addr)
				if err != nil || reply == nil || reply.Rcode != dns.RcodeSuccess {
					t.Fatalf("%s %s: reply=%v, error=%v", tc.proto, tc.name, reply, err)
				}
				req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, metricsURL, nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Accept", "application/openmetrics-text; version=1.0.0")
				resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/openmetrics-text") {
					t.Fatalf("scrape status = %d, Content-Type = %q", resp.StatusCode, resp.Header.Get("Content-Type"))
				}
				found := false
				for line := range strings.SplitSeq(string(body), "\n") {
					if !strings.HasPrefix(line, "coredns_proxy_request_duration_seconds_bucket{") || !strings.Contains(line, `to="`+upstreamAddr+`"`) {
						continue
					}
					if strings.Contains(line, `query_name="`+strings.ToLower(tc.name)+`"`) && strings.Contains(line, `query_type="`+dns.Type(tc.qtype).String()+`"`) {
						found = true
					}
				}
				if want := enabled && tc.wantSample; found != want {
					t.Fatalf("%s %s: exemplar present = %v, want %v", tc.proto, tc.name, found, want)
				}
			}
			families, err := prometheus.DefaultGatherer.Gather()
			if err != nil {
				t.Fatal(err)
			}
			var found int
			for _, family := range families {
				if family.GetName() != "coredns_proxy_request_duration_seconds" {
					continue
				}
				for _, metric := range family.GetMetric() {
					labels := make(map[string]string)
					for _, pair := range metric.GetLabel() {
						labels[pair.GetName()] = pair.GetValue()
					}
					if labels["proxy_name"] != "forward" || labels["to"] != upstreamAddr {
						continue
					}
					found++
					if len(labels) != 3 || labels["rcode"] != "NOERROR" || metric.GetHistogram().GetSampleCount() != uint64(len(cases)) {
						t.Fatalf("ordinary histogram changed: %v", metric)
					}
				}
			}
			if found != 1 {
				t.Fatalf("histogram series = %d, want 1", found)
			}
		})
	}
}
