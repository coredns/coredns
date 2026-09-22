package auto

import (
	"testing"

	"github.com/coredns/coredns/plugin/file"
)

func TestZonesNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		zones    []string
		expected []string
	}{
		{
			name:     "empty zones",
			zones:    []string{},
			expected: []string{},
		},
		{
			name:     "single zone",
			zones:    []string{"example.org."},
			expected: []string{"example.org."},
		},
		{
			name:     "multiple zones",
			zones:    []string{"example.org.", "test.org.", "another.com."},
			expected: []string{"example.org.", "test.org.", "another.com."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			z := &Zones{
				names: tt.zones,
			}

			result := z.Names()

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d names, got %d", len(tt.expected), len(result))
			}

			for i, name := range tt.expected {
				if i >= len(result) || result[i] != name {
					t.Errorf("Expected name %s at index %d, got %s", name, i, result[i])
				}
			}
		})
	}
}

func TestZonesOrigins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		origins  []string
		expected []string
	}{
		{
			name:     "empty origins",
			origins:  []string{},
			expected: []string{},
		},
		{
			name:     "single origin",
			origins:  []string{"example.org."},
			expected: []string{"example.org."},
		},
		{
			name:     "multiple origins",
			origins:  []string{"example.org.", "test.org."},
			expected: []string{"example.org.", "test.org."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			z := &Zones{
				origins: tt.origins,
			}

			result := z.Origins()

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d origins, got %d", len(tt.expected), len(result))
			}

			for i, origin := range tt.expected {
				if i >= len(result) || result[i] != origin {
					t.Errorf("Expected origin %s at index %d, got %s", origin, i, result[i])
				}
			}
		})
	}
}

func TestZonesZones(t *testing.T) {
	t.Parallel()

	zone1 := &file.Zone{}
	zone2 := &file.Zone{}

	z := &Zones{
		Z: map[string]*file.Zone{
			"example.org.": zone1,
			"test.org.":    zone2,
		},
	}

	tests := []struct {
		name     string
		zoneName string
		expected *file.Zone
	}{
		{
			name:     "existing zone",
			zoneName: "example.org.",
			expected: zone1,
		},
		{
			name:     "another existing zone",
			zoneName: "test.org.",
			expected: zone2,
		},
		{
			name:     "non-existent zone",
			zoneName: "notfound.org.",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := z.Zones(tt.zoneName)

			if result != tt.expected {
				t.Errorf("Expected zone %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestZonesAdd(t *testing.T) {
	t.Parallel()

	z := &Zones{}
	zone := &file.Zone{}

	// Test adding to empty zones
	z.Add(zone, "example.org.", nil)

	if z.Z == nil {
		t.Error("Expected Z map to be initialized")
	}

	if z.Z["example.org."] != zone {
		t.Error("Expected zone to be added to map")
	}

	if len(z.names) != 1 || z.names[0] != "example.org." {
		t.Errorf("Expected names to contain 'example.org.', got %v", z.names)
	}

	// Test adding another zone
	zone2 := &file.Zone{}
	z.Add(zone2, "test.org.", nil)

	if len(z.Z) != 2 {
		t.Errorf("Expected 2 zones in map, got %d", len(z.Z))
	}

	if z.Z["test.org."] != zone2 {
		t.Error("Expected second zone to be added to map")
	}

	if len(z.names) != 2 {
		t.Errorf("Expected 2 names, got %d", len(z.names))
	}
}

func TestZonesEmptyOperations(t *testing.T) {
	t.Parallel()

	z := &Zones{}

	names := z.Names()
	if len(names) != 0 {
		t.Errorf("Expected empty names slice, got %v", names)
	}

	origins := z.Origins()
	if len(origins) != 0 {
		t.Errorf("Expected empty origins slice, got %v", origins)
	}

	zone := z.Zones("any.zone.")
	if zone != nil {
		t.Errorf("Expected nil zone, got %v", zone)
	}

	z.Remove("any.zone.")

	testZone := &file.Zone{}
	z.Add(testZone, "test.org.", nil)

	if z.Z == nil {
		t.Error("Expected Z map to be initialized after Add")
	}
	if z.Z["test.org."] != testZone {
		t.Error("Expected zone to be added")
	}
}
