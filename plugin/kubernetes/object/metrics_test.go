package object

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	dto "github.com/prometheus/client_model/go"
	api "k8s.io/api/core/v1"
)

// histSampleCount reads the sample count for a specific service_kind label
// from a HistogramVec.
func histSampleCount(t *testing.T, vec *prometheus.HistogramVec, label string) uint64 {
	t.Helper()
	obs, err := vec.GetMetricWithLabelValues(label)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues(%q): %v", label, err)
	}
	m, ok := obs.(prometheus.Metric)
	if !ok {
		t.Fatalf("observer for label %q does not implement prometheus.Metric", label)
	}
	pb := &dto.Metric{}
	if err := m.Write(pb); err != nil {
		t.Fatalf("Write metric for label %q: %v", label, err)
	}
	return pb.GetHistogram().GetSampleCount()
}

// NOTE: subtests in this function must NOT call t.Parallel() — they swap
// global package-level vars (DNSProgrammingLatency, DurationSinceFunc).
func TestEndpointLatencyRecorder_record(t *testing.T) {
	tests := []struct {
		name            string
		services        []*Service
		ttSet           bool
		wantLabel       string
		wantSampleCount uint64
	}{
		{
			name:            "headless_with_selector: headless service with trigger annotation",
			services:        []*Service{{ClusterIPs: []string{api.ClusterIPNone}}},
			ttSet:           true,
			wantLabel:       "headless_with_selector",
			wantSampleCount: 1,
		},
		{
			name:            "cluster_ip: ClusterIP service with trigger annotation",
			services:        []*Service{{ClusterIPs: []string{"10.0.0.1"}}},
			ttSet:           true,
			wantLabel:       "cluster_ip",
			wantSampleCount: 1,
		},
		{
			name:            "no annotation on headless: TT zero means no observation",
			services:        []*Service{{ClusterIPs: []string{api.ClusterIPNone}}},
			ttSet:           false,
			wantLabel:       "headless_with_selector",
			wantSampleCount: 0,
		},
		{
			name:            "no annotation on ClusterIP: TT zero means no observation",
			services:        []*Service{{ClusterIPs: []string{"10.0.0.1"}}},
			ttSet:           false,
			wantLabel:       "cluster_ip",
			wantSampleCount: 0,
		},
		{
			name:            "informer lag: no backing service found, TT set, no observation",
			services:        nil,
			ttSet:           true,
			wantLabel:       "cluster_ip",
			wantSampleCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Replace global metric with a fresh unregistered histogram for isolation.
			// Do NOT add t.Parallel() here — these subtests swap global package state.
			origMetric := DNSProgrammingLatency
			reg := prometheus.NewRegistry()
			DNSProgrammingLatency = promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
				Name:    "test_dns_programming_duration_seconds",
				Help:    "test histogram",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 20),
			}, []string{"service_kind"})
			t.Cleanup(func() { DNSProgrammingLatency = origMetric })

			origDurationSince := DurationSinceFunc
			DurationSinceFunc = func(time.Time) time.Duration { return time.Second }
			t.Cleanup(func() { DurationSinceFunc = origDurationSince })

			rec := &EndpointLatencyRecorder{Services: tc.services}
			if tc.ttSet {
				rec.TT = time.Now().Add(-time.Second)
			}

			rec.record()

			got := histSampleCount(t, DNSProgrammingLatency, tc.wantLabel)
			if got != tc.wantSampleCount {
				t.Errorf("sample count for label %q = %d, want %d", tc.wantLabel, got, tc.wantSampleCount)
			}
		})
	}
}
