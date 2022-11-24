package opentelemetry

import (
	"testing"
	"time"

	"github.com/coredns/caddy"

	tracesdk "go.opentelemetry.io/otel/sdk/trace"
)

func TestOpentelemetryParse(t *testing.T) {
	tests := []struct {
		input               string
		wantErr             bool
		endpoint            string
		endpointType        string
		serviceName         string
		samplingProbability float64
		insecure            bool
		batchSpanOptions    tracesdk.BatchSpanProcessorOptions
	}{
		// valid
		{
			input:               `opentelemetry zipkin localhost:1234`,
			endpoint:            "http://localhost:1234/api/v2/spans",
			endpointType:        "zipkin",
			serviceName:         "coredns",
			samplingProbability: 1,
			batchSpanOptions:    tracesdk.BatchSpanProcessorOptions{MaxQueueSize: 2048, BatchTimeout: 5 * time.Second, ExportTimeout: 30 * time.Second, MaxExportBatchSize: 512},
		},
		{
			input:               `opentelemetry localhost:1234`,
			endpoint:            "http://localhost:1234/api/v2/spans",
			endpointType:        "zipkin",
			serviceName:         "coredns",
			samplingProbability: 1,
			batchSpanOptions:    tracesdk.BatchSpanProcessorOptions{MaxQueueSize: 2048, BatchTimeout: 5 * time.Second, ExportTimeout: 30 * time.Second, MaxExportBatchSize: 512},
		},
		{
			input:               `opentelemetry zipkin http://localhost:9411/api/v2/spans`,
			endpoint:            "http://localhost:9411/api/v2/spans",
			endpointType:        "zipkin",
			serviceName:         "coredns",
			samplingProbability: 1,
			batchSpanOptions:    tracesdk.BatchSpanProcessorOptions{MaxQueueSize: 2048, BatchTimeout: 5 * time.Second, ExportTimeout: 30 * time.Second, MaxExportBatchSize: 512},
		},
		{
			input:               "opentelemetry zipkin localhost:1234 {\nservice test_service\n}",
			endpoint:            "http://localhost:1234/api/v2/spans",
			endpointType:        "zipkin",
			serviceName:         "test_service",
			samplingProbability: 1,
			batchSpanOptions:    tracesdk.BatchSpanProcessorOptions{MaxQueueSize: 2048, BatchTimeout: 5 * time.Second, ExportTimeout: 30 * time.Second, MaxExportBatchSize: 512},
		},
		{
			input:               "opentelemetry zipkin localhost:1234 {\n service test_service\n sampling_probability 0.5\n}",
			endpoint:            "http://localhost:1234/api/v2/spans",
			endpointType:        "zipkin",
			serviceName:         "test_service",
			samplingProbability: 0.5,
			batchSpanOptions:    tracesdk.BatchSpanProcessorOptions{MaxQueueSize: 2048, BatchTimeout: 5 * time.Second, ExportTimeout: 30 * time.Second, MaxExportBatchSize: 512},
		},
		{
			input:               "opentelemetry zipkin localhost:1234 {\n service test_service\n sampling_probability 0.5\n insecure\n}",
			endpoint:            "http://localhost:1234/api/v2/spans",
			endpointType:        "zipkin",
			serviceName:         "test_service",
			samplingProbability: 0.5,
			insecure:            true,
			batchSpanOptions:    tracesdk.BatchSpanProcessorOptions{MaxQueueSize: 2048, BatchTimeout: 5 * time.Second, ExportTimeout: 30 * time.Second, MaxExportBatchSize: 512},
		},
		{
			input:               "opentelemetry zipkin localhost:1234 {\n service test_service\n insecure\n max_queue_size 10\n batch_timeout 20s\n export_timeout 50s\n max_export_batch_size 25\n}",
			endpoint:            "http://localhost:1234/api/v2/spans",
			endpointType:        "zipkin",
			serviceName:         "test_service",
			samplingProbability: 1,
			insecure:            true,
			batchSpanOptions:    tracesdk.BatchSpanProcessorOptions{MaxQueueSize: 10, BatchTimeout: 20 * time.Second, ExportTimeout: 50 * time.Second, MaxExportBatchSize: 25},
		},
		{
			input:               `opentelemetry otelhttp 1.2.3.4:9874`,
			endpoint:            "1.2.3.4:9874",
			endpointType:        "otelhttp",
			serviceName:         "coredns",
			samplingProbability: 1,
			batchSpanOptions:    tracesdk.BatchSpanProcessorOptions{MaxQueueSize: 2048, BatchTimeout: 5 * time.Second, ExportTimeout: 30 * time.Second, MaxExportBatchSize: 512},
		},
		{
			input:               "opentelemetry otelhttp 1.2.3.4:9874 {\n service awesome_service\n insecure\n sampling_probability 0.75\n}",
			endpoint:            "1.2.3.4:9874",
			endpointType:        "otelhttp",
			serviceName:         "awesome_service",
			samplingProbability: 0.75,
			insecure:            true,
			batchSpanOptions:    tracesdk.BatchSpanProcessorOptions{MaxQueueSize: 2048, BatchTimeout: 5 * time.Second, ExportTimeout: 30 * time.Second, MaxExportBatchSize: 512},
		},
		{
			input:               "opentelemetry otelhttp 1.2.3.4:9874 {\n service test_service\n sampling_probability 0.5\n insecure\n batch_timeout 123s\n}",
			endpoint:            "1.2.3.4:9874",
			endpointType:        "otelhttp",
			serviceName:         "test_service",
			samplingProbability: 0.5,
			insecure:            true,
			batchSpanOptions:    tracesdk.BatchSpanProcessorOptions{MaxQueueSize: 2048, BatchTimeout: 123 * time.Second, ExportTimeout: 30 * time.Second, MaxExportBatchSize: 512},
		},
		{
			input:               "opentelemetry otelhttp 1.2.3.4:9874 {\n service testing\n insecure\n max_queue_size 100\n batch_timeout 28s\n export_timeout 42s\n max_export_batch_size 25\n}",
			endpoint:            "1.2.3.4:9874",
			endpointType:        "otelhttp",
			serviceName:         "testing",
			samplingProbability: 1,
			insecure:            true,
			batchSpanOptions:    tracesdk.BatchSpanProcessorOptions{MaxQueueSize: 100, BatchTimeout: 28 * time.Second, ExportTimeout: 42 * time.Second, MaxExportBatchSize: 25},
		},

		// invalid
		{
			input:               `opentelemetry wrong localhost:1234`,
			wantErr:             true,
			samplingProbability: 1,
		},
		{
			input:               `opentelemetry otelhttp http://1.2.3.4:9874`,
			wantErr:             true,
			samplingProbability: 1,
		},
		{
			input:               "opentelemetry zipkin localhost:1234 {\n sampling_probability 2\n}",
			wantErr:             true,
			samplingProbability: 1,
		},
		{
			input:               "opentelemetry otelhttp localhost:1234 {\n max_queue_size wrong\n}",
			wantErr:             true,
			samplingProbability: 1,
		},
		{
			input:               "opentelemetry otelhttp localhost:1234 {\n batch_timeout wrong\n}",
			wantErr:             true,
			samplingProbability: 1,
		},
		{
			input:               "opentelemetry otelhttp localhost:1234 {\n export_timeout wrong\n}",
			wantErr:             true,
			samplingProbability: 1,
		},
		{
			input:               "opentelemetry otelhttp localhost:1234 {\n max_export_batch_size wrong\n}",
			wantErr:             true,
			samplingProbability: 1,
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		cfg, err := opentelemetryParse(c)
		if test.wantErr && err == nil {
			t.Errorf("Test %v: Expected error but found nil", i)
			continue
		} else if !test.wantErr && err != nil {
			t.Errorf("Test %v: Expected no error but found error: %v", i, err)
			continue
		}

		if test.wantErr {
			continue
		}

		if cfg.Endpoint != test.endpoint {
			t.Errorf("Test %v: Expected endpoint to be %s but found: %s", i, test.endpoint, cfg.Endpoint)
		}

		if cfg.EndpointType != test.endpointType {
			t.Errorf("Test %v: Expected endpointType to be %s but found: %s", i, test.endpointType, cfg.EndpointType)
		}

		if cfg.serviceName != test.serviceName {
			t.Errorf("Test %v: Expected serviceName to be %s but found: %s", i, test.serviceName, cfg.serviceName)
		}

		if cfg.SamplingProbability != test.samplingProbability {
			t.Errorf("Test %v: Expected samplingProbability to be %f but found: %f", i, test.samplingProbability, cfg.SamplingProbability)
		}

		if cfg.Insecure != test.insecure {
			t.Errorf("Test %v: Expected insecure to be %v but found: %v", i, test.insecure, cfg.Insecure)
		}

		if cfg.batchSpanOptions.MaxQueueSize != test.batchSpanOptions.MaxQueueSize {
			t.Errorf("Test %v: Expected maxQueueSize to be %d but found: %d", i, test.batchSpanOptions.MaxQueueSize, cfg.batchSpanOptions.MaxQueueSize)
		}

		if cfg.batchSpanOptions.BatchTimeout != test.batchSpanOptions.BatchTimeout {
			t.Errorf("Test %v: Expected batchTimeout to be %d but found: %d", i, test.batchSpanOptions.BatchTimeout, cfg.batchSpanOptions.BatchTimeout)
		}

		if cfg.batchSpanOptions.ExportTimeout != test.batchSpanOptions.ExportTimeout {
			t.Errorf("Test %v: Expected exportTimeout to be %s but found: %s", i, test.batchSpanOptions.ExportTimeout, cfg.batchSpanOptions.ExportTimeout)
		}

		if cfg.batchSpanOptions.MaxExportBatchSize != test.batchSpanOptions.MaxExportBatchSize {
			t.Errorf("Test %v: Expected maxExportBatchSize to be %d but found: %d", i, test.batchSpanOptions.MaxExportBatchSize, cfg.batchSpanOptions.MaxExportBatchSize)
		}
	}
}
