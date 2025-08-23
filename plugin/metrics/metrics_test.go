package metrics

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestMetrics(t *testing.T) {
	met := New("localhost:0")
	if err := met.OnStartup(); err != nil {
		t.Fatalf("Failed to start metrics handler: %s", err)
	}
	defer met.OnFinalShutdown()

	met.AddZone("example.org.")

	tests := []struct {
		next          plugin.Handler
		qname         string
		qtype         uint16
		metric        string
		expectedValue string
	}{
		// This all works because 1 bucket (1 zone, 1 type)
		{
			next:          test.NextHandler(dns.RcodeSuccess, nil),
			qname:         "example.org.",
			metric:        "coredns_dns_requests_total",
			expectedValue: "1",
		},
		{
			next:          test.NextHandler(dns.RcodeSuccess, nil),
			qname:         "example.org.",
			metric:        "coredns_dns_requests_total",
			expectedValue: "2",
		},
		{
			next:          test.NextHandler(dns.RcodeSuccess, nil),
			qname:         "example.org.",
			metric:        "coredns_dns_requests_total",
			expectedValue: "3",
		},
		{
			next:          test.NextHandler(dns.RcodeSuccess, nil),
			qname:         "example.org.",
			metric:        "coredns_dns_responses_total",
			expectedValue: "4",
		},
	}

	ctx := context.TODO()

	for i, tc := range tests {
		req := new(dns.Msg)
		if tc.qtype == 0 {
			tc.qtype = dns.TypeA
		}
		req.SetQuestion(tc.qname, tc.qtype)
		met.Next = tc.next

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := met.ServeDNS(ctx, rec, req)
		if err != nil {
			t.Fatalf("Test %d: Expected no error, but got %s", i, err)
		}

		result := test.Scrape("http://" + ListenAddr + "/metrics")

		if tc.expectedValue != "" {
			got, _ := test.MetricValue(tc.metric, result)
			if got != tc.expectedValue {
				t.Errorf("Test %d: Expected value %s for metrics %s, but got %s", i, tc.expectedValue, tc.metric, got)
			}
		}
	}
}

func TestMetricsHTTPTimeout(t *testing.T) {
	met := New("localhost:0")
	if err := met.OnStartup(); err != nil {
		t.Fatalf("Failed to start metrics handler: %s", err)
	}
	defer met.OnFinalShutdown()

	// Use context with timeout to prevent test from hanging indefinitely
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan error, 1)

	go func() {
		conn, err := net.Dial("tcp", ListenAddr)
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()

		// Send partial HTTP request and then stop sending data
		// This will cause the server to wait for more data and hit ReadTimeout
		partialRequest := "GET /metrics HTTP/1.1\r\nHost: " + ListenAddr + "\r\nContent-Length: 100\r\n\r\n"
		_, err = conn.Write([]byte(partialRequest))
		if err != nil {
			done <- err
			return
		}

		// Now just wait - server should timeout trying to read the remaining data
		// If server has no ReadTimeout, this will hang indefinitely
		buffer := make([]byte, 1024)
		_, err = conn.Read(buffer)
		done <- err
	}()

	select {
	case <-done:
		t.Log("HTTP request timed out by server")
	case <-ctx.Done():
		t.Error("HTTP request did not time out")
	}
}
