package dynamicforward

import (
	"github.com/coredns/caddy"
	"testing"
	"time"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expected      DynamicForwardConfig
		expectErr     bool
		expectedError string
	}{
		{
			name: "Valid config with all parameters",
			input: `dynamicforward {
				namespace kube-system
				service_name d8-kube-dns
				port_name dns
				expire 10m
				health_check 5s
			}`,
			expected: DynamicForwardConfig{
				Namespace:   "kube-system",
				ServiceName: "d8-kube-dns",
				PortName:    "dns",
				Expire:      10 * time.Minute,
				HealthCheck: 5 * time.Second,
			},
			expectErr: false,
		},
		{
			name: "Config missing namespace",
			input: `dynamicforward {
				service_name d8-kube-dns
				port_name dns
				expire 10m
				health_check 5s
			}`,
			expectErr:     true,
			expectedError: "namespace, servicename, and portname are required parameters",
		},
		{
			name: "Config with invalid expire value",
			input: `dynamicforward {
				namespace kube-system
				service_name d8-kube-dns
				port_name dns
				expire not-a-duration
			}`,
			expectErr:     true,
			expectedError: "invalid expire duration",
		},
		{
			name: "Config with missing health_check value",
			input: `dynamicforward {
				namespace kube-system
				service_name d8-kube-dns
				port_name dns
				health_check 
			}`,
			expectErr:     true,
			expectedError: "wrong argument count or unexpected line ending",
		},
		{
			name: "Minimal valid config",
			input: `dynamicforward {
				namespace kube-system
				service_name d8-kube-dns
				port_name dns
			}`,
			expected: DynamicForwardConfig{
				Namespace:   "kube-system",
				ServiceName: "d8-kube-dns",
				PortName:    "dns",
				Expire:      30 * time.Minute,
				HealthCheck: 10 * time.Second,
			},
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Parsing controller
			controller := caddy.NewTestController("dns", test.input)

			// Parse config
			config, err := ParseConfig(controller)

			// Check err
			if test.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !containsError(err.Error(), test.expectedError) {
					t.Fatalf("expected error containing %q, got %q", test.expectedError, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Compare results
			if config.Namespace != test.expected.Namespace {
				t.Errorf("expected namespace %q, got %q", test.expected.Namespace, config.Namespace)
			}
			if config.ServiceName != test.expected.ServiceName {
				t.Errorf("expected label %q, got %q", test.expected.ServiceName, config.ServiceName)
			}
			if config.PortName != test.expected.PortName {
				t.Errorf("expected portname %q, got %q", test.expected.PortName, config.PortName)
			}
			if config.Expire != test.expected.Expire {
				t.Errorf("expected expire %v, got %v", test.expected.Expire, config.Expire)
			}
			if config.HealthCheck != test.expected.HealthCheck {
				t.Errorf("expected health_check %v, got %v", test.expected.HealthCheck, config.HealthCheck)
			}
		})
	}
}

// containsError
func containsError(actual, expected string) bool {
	return len(actual) >= len(expected) && actual[:len(expected)] == expected
}
