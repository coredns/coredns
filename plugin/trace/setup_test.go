package trace

import (
	"testing"
	"time"

	"github.com/coredns/caddy"
)

func TestTraceParse(t *testing.T) {
	tests := []struct {
		input                  string
		shouldErr              bool
		endpoint               string
		every                  uint64
		serviceName            string
		clientServer           bool
		zipkinMaxBacklogSize   int
		zipkinMaxBatchSize     int
		zipkinMaxBatchInterval time.Duration
	}{
		// oks
		{`trace`, false, "http://localhost:9411/api/v2/spans", 1, `coredns`, false, 0, 0, 0},
		{`trace localhost:1234`, false, "http://localhost:1234/api/v2/spans", 1, `coredns`, false, 0, 0, 0},
		{`trace http://localhost:1234/somewhere/else`, false, "http://localhost:1234/somewhere/else", 1, `coredns`, false, 0, 0, 0},
		{`trace zipkin localhost:1234`, false, "http://localhost:1234/api/v2/spans", 1, `coredns`, false, 0, 0, 0},
		{`trace datadog localhost`, false, "localhost", 1, `coredns`, false, 0, 0, 0},
		{`trace datadog http://localhost:8127`, false, "http://localhost:8127", 1, `coredns`, false, 0, 0, 0},
		{"trace datadog localhost {\n datadog_analytics_rate 0.1\n}", false, "localhost", 1, `coredns`, false, 0, 0, 0},
		{"trace {\n every 100\n}", false, "http://localhost:9411/api/v2/spans", 100, `coredns`, false, 0, 0, 0},
		{"trace {\n every 100\n service foobar\nclient_server\n}", false, "http://localhost:9411/api/v2/spans", 100, `foobar`, true, 0, 0, 0},
		{"trace {\n every 2\n client_server true\n}", false, "http://localhost:9411/api/v2/spans", 2, `coredns`, true, 0, 0, 0},
		{"trace {\n client_server false\n}", false, "http://localhost:9411/api/v2/spans", 1, `coredns`, false, 0, 0, 0},
		{"trace {\n zipkin_max_backlog_size 100\n zipkin_max_batch_size 200\n zipkin_max_batch_interval 10s\n}", false,
			"http://localhost:9411/api/v2/spans", 1, `coredns`, false, 100, 200, 10 * time.Second},

		// fails
		{`trace footype localhost:4321`, true, "", 1, "", false, 0, 0, 0},
		{"trace {\n every 2\n client_server junk\n}", true, "", 1, "", false, 0, 0, 0},
		{"trace datadog localhost {\n datadog_analytics_rate 2\n}", true, "", 1, "", false, 0, 0, 0},
		{"trace {\n zipkin_max_backlog_size wrong\n}", true, "", 1, `coredns`, false, 0, 0, 0},
		{"trace {\n zipkin_max_batch_size wrong\n}", true, "", 1, `coredns`, false, 0, 0, 0},
		{"trace {\n zipkin_max_batch_interval wrong\n}", true, "", 1, `coredns`, false, 0, 0, 0},
		{"trace {\n zipkin_max_backlog_size\n}", true, "", 1, `coredns`, false, 0, 0, 0},
		{"trace {\n zipkin_max_batch_size\n}", true, "", 1, `coredns`, false, 0, 0, 0},
		{"trace {\n zipkin_max_batch_interval\n}", true, "", 1, `coredns`, false, 0, 0, 0},
	}
	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		m, err := traceParse(c)
		if test.shouldErr && err == nil {
			t.Errorf("Test %v: Expected error but found nil", i)
			continue
		} else if !test.shouldErr && err != nil {
			t.Errorf("Test %v: Expected no error but found error: %v", i, err)
			continue
		}

		if test.shouldErr {
			continue
		}

		if m.serviceEndpoint != "" {
			t.Errorf("Test %v: Expected serviceEndpoint to be '' but found: %s", i, m.serviceEndpoint)
		}
		if test.endpoint != m.Endpoint {
			t.Errorf("Test %v: Expected endpoint %s but found: %s", i, test.endpoint, m.Endpoint)
		}
		if test.every != m.every {
			t.Errorf("Test %v: Expected every %d but found: %d", i, test.every, m.every)
		}
		if test.serviceName != m.serviceName {
			t.Errorf("Test %v: Expected service name %s but found: %s", i, test.serviceName, m.serviceName)
		}
		if test.clientServer != m.clientServer {
			t.Errorf("Test %v: Expected client_server %t but found: %t", i, test.clientServer, m.clientServer)
		}
		if test.zipkinMaxBacklogSize != m.zipkinMaxBacklogSize {
			t.Errorf("Test %v: Expected zipkin_max_backlog_size %d but found: %d", i, test.zipkinMaxBacklogSize, m.zipkinMaxBacklogSize)
		}
		if test.zipkinMaxBatchSize != m.zipkinMaxBatchSize {
			t.Errorf("Test %v: Expected zipkin_max_batch_size %d but found: %d", i, test.zipkinMaxBatchSize, m.zipkinMaxBatchSize)
		}
		if test.zipkinMaxBatchInterval != m.zipkinMaxBatchInterval {
			t.Errorf("Test %v: Expected zipkin_max_batch_interval %v but found: %v", i, test.zipkinMaxBatchInterval, m.zipkinMaxBatchInterval)
		}
	}
}
