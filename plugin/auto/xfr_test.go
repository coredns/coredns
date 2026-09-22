package auto

import (
	"testing"

	"github.com/coredns/coredns/plugin/file"
)

func TestAutoNotify(t *testing.T) {
	t.Parallel()

	a := &Auto{
		Zones: &Zones{
			names: []string{"example.org.", "test.org."},
		},
		transfer: nil,
	}

	err := a.Notify()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestAutoTransferZoneCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		zone        string
		expectError bool
		errorType   string
	}{
		{
			name:        "exact match",
			zone:        "example.org.",
			expectError: true,
			errorType:   "no SOA",
		},
		{
			name:        "case different",
			zone:        "EXAMPLE.ORG.",
			expectError: true,
			errorType:   "not authoritative",
		},
		{
			name:        "no match",
			zone:        "other.org.",
			expectError: true,
			errorType:   "not authoritative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := createTestAutoForTransfer(t, []string{"example.org."})

			ch, err := a.Transfer(tt.zone, 1234)

			if !tt.expectError {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
				if ch == nil {
					t.Error("Expected non-nil channel")
				}
			} else {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				if ch != nil {
					t.Error("Expected nil channel when error occurs")
				}
			}
		})
	}
}

// Helper functions

func createTestAutoForTransfer(t *testing.T, zones []string) *Auto {
	t.Helper()
	a := &Auto{
		Zones: &Zones{
			Z:     make(map[string]*file.Zone),
			names: zones,
		},
	}

	// Initialize with real empty zones for the tests
	for _, zone := range zones {
		a.Z[zone] = &file.Zone{}
	}

	return a
}
