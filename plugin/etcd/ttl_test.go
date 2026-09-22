package etcd

import (
	"testing"

	"github.com/coredns/coredns/plugin/etcd/msg"

	"go.etcd.io/etcd/api/v3/mvccpb"
)

func TestTTL(t *testing.T) {
	tests := []struct {
		name        string
		leaseID     int64
		serviceTTL  uint32
		minLeaseTTL uint32
		maxLeaseTTL uint32
		hasClient   bool
		expectedTTL uint32
	}{
		{
			name:        "no client, large lease ID falls back to default",
			leaseID:     0x12345678FFFFFFFF, // Large lease ID that would cause issues
			serviceTTL:  0,
			minLeaseTTL: 0,
			maxLeaseTTL: 0,
			hasClient:   false,
			expectedTTL: defaultTTL,
		},
		{
			name:        "no client, zero lease ID falls back to default",
			leaseID:     0,
			serviceTTL:  0,
			minLeaseTTL: 0,
			maxLeaseTTL: 0,
			hasClient:   false,
			expectedTTL: defaultTTL,
		},
		{
			name:        "no client, service TTL takes precedence",
			leaseID:     120,
			serviceTTL:  300,
			minLeaseTTL: 0,
			maxLeaseTTL: 0,
			hasClient:   false,
			expectedTTL: 300,
		},
		{
			name:        "no client, smaller service TTL wins",
			leaseID:     600,
			serviceTTL:  120,
			minLeaseTTL: 0,
			maxLeaseTTL: 0,
			hasClient:   false,
			expectedTTL: 120,
		},
		{
			name:        "custom bounds, no client",
			leaseID:     0x12345678FFFFFFFF,
			serviceTTL:  0,
			minLeaseTTL: 60,   // 1 minute
			maxLeaseTTL: 3600, // 1 hour
			hasClient:   false,
			expectedTTL: defaultTTL,
		},
		{
			name:        "zero service TTL with lease ID",
			leaseID:     600,
			serviceTTL:  0,
			minLeaseTTL: 0,
			maxLeaseTTL: 0,
			hasClient:   false,
			expectedTTL: defaultTTL,
		},
		{
			name:        "both zero, falls back to default",
			leaseID:     0,
			serviceTTL:  0,
			minLeaseTTL: 0,
			maxLeaseTTL: 0,
			hasClient:   false,
			expectedTTL: defaultTTL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create Etcd instance with test configuration
			e := &Etcd{
				MinLeaseTTL: tt.minLeaseTTL,
				MaxLeaseTTL: tt.maxLeaseTTL,
			}

			// Create test data
			kv := &mvccpb.KeyValue{
				Key:   []byte("/test/service"),
				Value: []byte(`{"host": "test.example.com"}`),
				Lease: tt.leaseID,
			}

			serv := &msg.Service{
				Host: "test.example.com",
				TTL:  tt.serviceTTL,
			}

			resultingTTL := e.TTL(kv, serv)

			if resultingTTL != tt.expectedTTL {
				t.Errorf("TTL() = %d, expected %d", resultingTTL, tt.expectedTTL)
			}
		})
	}
}
