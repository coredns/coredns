package plugin

import (
	"reflect"
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
		{"10.9.3.0/18", []string{"0.9.10.in-addr.arpa.", "1.9.10.in-addr.arpa.", "2.9.10.in-addr.arpa."}},
		{"2001:db8::/29", []string{
			"8.b.d.0.1.0.0.2.ip6.arpa.",
			"9.b.d.0.1.0.0.2.ip6.arpa.",
			"a.b.d.0.1.0.0.2.ip6.arpa.",
			"b.b.d.0.1.0.0.2.ip6.arpa.",
			"c.b.d.0.1.0.0.2.ip6.arpa.",
			"d.b.d.0.1.0.0.2.ip6.arpa.",
			"e.b.d.0.1.0.0.2.ip6.arpa.",
			"f.b.d.0.1.0.0.2.ip6.arpa.",
		}},
		{"2001:db8::/30", []string{
			"8.b.d.0.1.0.0.2.ip6.arpa.",
			"9.b.d.0.1.0.0.2.ip6.arpa.",
			"a.b.d.0.1.0.0.2.ip6.arpa.",
			"b.b.d.0.1.0.0.2.ip6.arpa.",
		}},
		{"2001:db8::/115", []string{
			"0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
		}},
		{"2001:db8::/114", []string{
			"0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"2.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"3.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
		}},
		{"2001:db8::/113", []string{
			"0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"2.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"3.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"4.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"6.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"7.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
		}},
		{"2001:db8::/112", []string{
			"0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
		}},
		{"2001:db8::/108", []string{
			"0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
		}},
		{"::fFFF:B:F/115", nil},
		{"dns://example.org", []string{"example.org."}},
	}

	for i := range tests {
		actual := Host(tests[i].in).NormalizeExact()
		expected := tests[i].out
		sort.Strings(expected)
		for j := range expected {
			if expected[j] != actual[j] {
				t.Errorf("Test %d, expected %v, got %v", i, expected[j], actual[j])
			}
		}
	}
}

func TestOriginsFromArgsOrServerBlock(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		serverblock []string
		expected    []string
	}{
		{
			name:        "args",
			args:        []string{"example.org"},
			serverblock: []string{"ignored.local"},
			expected:    []string{"example.org."},
		},
		{
			name:        "args with cidr expands",
			args:        []string{"10.0.0.0/15"},
			serverblock: nil,
			expected:    []string{"0.10.in-addr.arpa.", "1.10.in-addr.arpa."},
		},
		{
			name:        "serverblock first normalized",
			args:        nil,
			serverblock: []string{"example.org"},
			expected:    []string{"example.org."},
		},
		{
			name:        "serverblock cidr first only",
			args:        nil,
			serverblock: []string{"10.0.0.0/15"},
			expected:    []string{"0.10.in-addr.arpa."},
		},
		{
			name:        "serverblock invalid utf-8 preserved",
			args:        nil,
			serverblock: []string{"\xFF\n:", "example.org"},
			expected:    []string{"\xFF\n:", "example.org."},
		},
		{
			name:        "args invalid utf-8 dropped",
			args:        []string{"\xFF\n:", "example.org"},
			serverblock: nil,
			expected:    []string{"example.org."},
		},
		{
			name:        "serverblock invalid utf-8 with prefix",
			args:        nil,
			serverblock: []string{"unix://\xff\netcd", "example.org"},
			expected:    []string{"\uFFFD\netcd.", "example.org."}, // \uFFFD is the replacement character
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OriginsFromArgsOrServerBlock(tt.args, tt.serverblock)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("%s: expected %q, got %q", tt.name, tt.expected, got)
			}
		})
	}
}
