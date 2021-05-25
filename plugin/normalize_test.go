package plugin

import (
	"sort"
	"testing"
)

func TestZoneMatches(t *testing.T) {
	child := "example.org."
	zones := Zones([]string{"org.", "."})
	actual := zones.Matches(child)
	if actual != "org." {
		t.Errorf("Expected %v, got %v", "org.", actual)
	}

	child = "bla.example.org."
	zones = Zones([]string{"bla.example.org.", "org.", "."})
	actual = zones.Matches(child)

	if actual != "bla.example.org." {
		t.Errorf("Expected %v, got %v", "org.", actual)
	}
}

func TestZoneNormalize(t *testing.T) {
	zones := Zones([]string{"example.org", "Example.ORG.", "example.org."})
	expected := "example.org."
	zones.Normalize()

	for _, actual := range zones {
		if actual != expected {
			t.Errorf("Expected %v, got %v", expected, actual)
		}
	}
}

func TestNameMatches(t *testing.T) {
	matches := []struct {
		child    string
		parent   string
		expected bool
	}{
		{".", ".", true},
		{"example.org.", ".", true},
		{"example.org.", "example.org.", true},
		{"example.org.", "org.", true},
		{"org.", "example.org.", false},
	}

	for _, m := range matches {
		actual := Name(m.parent).Matches(m.child)
		if actual != m.expected {
			t.Errorf("Expected %v for %s/%s, got %v", m.expected, m.parent, m.child, actual)
		}

	}
}

func TestNameNormalize(t *testing.T) {
	names := []string{
		"example.org", "example.org.",
		"Example.ORG.", "example.org."}

	for i := 0; i < len(names); i += 2 {
		ts := names[i]
		expected := names[i+1]
		actual := Name(ts).Normalize()
		if expected != actual {
			t.Errorf("Expected %v, got %v", expected, actual)
		}
	}
}

func TestHostNormalizeExact(t *testing.T) {
	tests := []struct {
		in  string
		out []string
	}{
		{".:53", []string{"."}},
		{"example.org:53", []string{"example.org."}},
		{"example.org.:53", []string{"example.org."}},
		{"10.0.0.0/8:53", []string{"10.in-addr.arpa."}},
		{"10.0.0.0/15", []string{"0.10.in-addr.arpa.", "1.10.in-addr.arpa."}},
		{"dns://example.org", []string{"example.org."}},
	}

	for i := range tests {
		actual := Host(tests[i].in).NormalizeExact()
		expected := tests[i].out
		sort.Strings(expected)
		for j := range expected {
			if expected[j] != actual[j] {
				t.Errorf("Test %d, expected %v, got %v", i, expected, actual)
			}
		}
	}
}
