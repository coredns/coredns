package nomad

import (
	"testing"

	"github.com/coredns/caddy"

	nomad "github.com/hashicorp/nomad/api"
)

func TestSetupNomad(t *testing.T) {
	tests := []struct {
		name           string
		config         string
		shouldErr      bool
		expectedFilter string
		expectedTTL    uint32
	}{
		{
			name: "valid_config_default_ttl",
			config: `
nomad service.nomad {
    address http://127.0.0.1:4646
    token test-token
}`,
			shouldErr:      false,
			expectedFilter: "",
			expectedTTL:    uint32(defaultTTL),
		},
		{
			name: "valid_config_custom_filter_and_ttl",
			config: `
nomad service.nomad {
    address http://127.0.0.1:4646
	filter "Tags not contains candidate"
    token test-token
    ttl 60
}`,
			shouldErr:      false,
			expectedFilter: "Tags not contains candidate",
			expectedTTL:    60,
		},
		{
			name: "invalid_ttl_negative",
			config: `
nomad service.nomad {
    address http://127.0.0.1:4646
    token test-token
    ttl -1
}`,
			shouldErr: true,
		},
		{
			name: "invalid_ttl_too_large",
			config: `
nomad service.nomad {
    address http://127.0.0.1:4646
    token test-token
    ttl 3601
}`,
			shouldErr: true,
		},
		{
			name: "invalid_property",
			config: `
nomad service.nomad {
    address http://127.0.0.1:4646
    token test-token
    invalid_property
}`,
			shouldErr: true,
		},
		{
			name: "multiple_addresses",
			config: `
nomad service.nomad {
    address http://127.0.0.1:4646 http://127.0.0.2:4646
    token test-token
}`,
			shouldErr:   false,
			expectedTTL: uint32(defaultTTL),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tt.config)
			n := &Nomad{
				ttl:     uint32(defaultTTL),
				clients: make([]*nomad.Client, 0),
				current: -1,
				filter:  "",
			}

			err := parse(c, n)
			if tt.shouldErr && err == nil {
				t.Fatalf("Test %s: expected error but got none", tt.name)
			}
			if !tt.shouldErr && err != nil {
				t.Fatalf("Test %s: expected no error but got: %v", tt.name, err)
			}
			if tt.shouldErr {
				return
			}

			if n.ttl != tt.expectedTTL {
				t.Errorf("Test %s: expected TTL %d, got %d", tt.name, tt.expectedTTL, n.ttl)
			}

			if len(n.clients) == 0 {
				t.Errorf("Test %s: expected at least one client to be created", tt.name)
			}
		})
	}
}
