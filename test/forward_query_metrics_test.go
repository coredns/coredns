package test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/dnstest"

	"github.com/miekg/dns"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

func TestForwardQueryMetrics(t *testing.T) {
	// Keep this fixture out of the process-wide request-size metrics inspected
	// by other integration tests. Proxy observations still use the real endpoint.
	collector := metrics.New("127.0.0.1:0")
	previousAddr := metrics.ListenAddr
	if err := collector.OnStartup(); err != nil {
		t.Fatal(err)
	}
	metricsURL := "http://" + metrics.ListenAddr + "/metrics"
	metrics.ListenAddr = previousAddr
	t.Cleanup(func() { _ = collector.OnFinalShutdown() })

	type series struct{ name, qtype, rcode string }
	readCounts := func(t *testing.T, upstream string) map[series]uint64 {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, metricsURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Accept", "text/plain; version=0.0.4")
		resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("metrics scrape: status=%d error=%v", resp.StatusCode, err)
		}
		parser := expfmt.NewTextParser(model.UTF8Validation)
		families, err := parser.TextToMetricFamilies(strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		counts := make(map[series]uint64)
		family := families["coredns_proxy_request_duration_seconds"]
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string)
			for _, pair := range metric.GetLabel() {
				labels[pair.GetName()] = pair.GetValue()
			}
			if labels["proxy_name"] != "forward" || labels["to"] != upstream {
				continue
			}
			wantLabels := 3
			if labels["query_name"] != "" {
				wantLabels = 5
			}
			if len(labels) != wantLabels {
				t.Fatalf("unexpected histogram labels: %v", labels)
			}
			key := series{labels["query_name"], labels["query_type"], labels["rcode"]}
			counts[key] += metric.GetHistogram().GetSampleCount()
		}
		return counts
	}

	for _, enabled := range []bool{false, true} {
		upstream := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
			response := new(dns.Msg).SetReply(r)
			if strings.HasPrefix(r.Question[0].Name, "missing.") {
				response.Rcode = dns.RcodeNameError
			}
			_ = w.WriteMsg(response)
		})
		// Retain upstreams until both modes finish, so their ports cannot overlap.
		t.Cleanup(upstream.Close)
		t.Run(fmt.Sprintf("enabled=%v", enabled), func(t *testing.T) {
			_, port, err := net.SplitHostPort(upstream.Addr)
			if err != nil {
				t.Fatal(err)
			}
			upstreamAddr := net.JoinHostPort("127.0.0.1", port)
			option := ""
			if enabled {
				option = "qtype_for_metrics A AAAA TYPE65400"
			}
			corefile := fmt.Sprintf(`.:0 {
	bind 127.0.0.1
	forward cluster.local %s {
		%s
	}
	forward . %s
}`, upstreamAddr, option, upstreamAddr)
			inst, udp, tcp, err := CoreDNSServerAndPorts(corefile)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = inst.Stop()
				inst.ShutdownCallbacks()
			})
			before := readCounts(t, upstreamAddr)
			cases := []struct {
				name     string
				qtype    uint16
				detailed bool
				rcode    int
			}{
				{"API.svc.CLUSTER.local.", dns.TypeA, true, dns.RcodeSuccess},
				{"api.svc.cluster.local.", dns.TypeA, true, dns.RcodeSuccess},
				{"api.svc.cluster.local.", dns.TypeAAAA, true, dns.RcodeSuccess},
				{"missing.cluster.local.", dns.TypeA, true, dns.RcodeNameError},
				{"cluster.local.", 65400, true, dns.RcodeSuccess},
				{"public.example.", dns.TypeA, false, dns.RcodeSuccess},
				{"notcluster.local.", dns.TypeA, false, dns.RcodeSuccess},
				{"txt.cluster.local.", dns.TypeTXT, false, dns.RcodeSuccess},
				{"any.cluster.local.", dns.TypeANY, false, dns.RcodeSuccess},
				{strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 47) + ".cluster.local.", dns.TypeA, true, dns.RcodeSuccess},
			}
			want := make(map[series]uint64)
			for range 2 {
				for _, protocol := range []string{"udp", "tcp"} {
					addr := udp
					if protocol == "tcp" {
						addr = tcp
					}
					client := &dns.Client{Net: protocol, Timeout: 2 * time.Second}
					for _, tc := range cases {
						reply, _, err := client.Exchange(new(dns.Msg).SetQuestion(tc.name, tc.qtype), addr)
						if err != nil || reply == nil || reply.Rcode != tc.rcode {
							t.Fatalf("%s %s: reply=%v error=%v", protocol, tc.name, reply, err)
						}
						key := series{rcode: dns.RcodeToString[tc.rcode]}
						if enabled && tc.detailed {
							key.name, key.qtype = strings.ToLower(tc.name), dns.Type(tc.qtype).String()
						}
						want[key]++
					}
				}
			}
			got := readCounts(t, upstreamAddr)
			for key, count := range before {
				got[key] -= count
				if got[key] == 0 {
					delete(got, key)
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("histogram counts = %v, want %v", got, want)
			}
		})
	}
}
