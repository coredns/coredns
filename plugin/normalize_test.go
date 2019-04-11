package plugin

import (
	"strconv"
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

func TestHostNormalize(t *testing.T) {
	normalizedNonMod8Cidr := []string{}
	for i := 0 ; i < 128 ; i++ {
		normalizedNonMod8Cidr = append(normalizedNonMod8Cidr, strconv.Itoa(i) + ".10.in-addr.arpa.")
	}
	for i, test := range []struct {
		input string
		expected []string
	}{
		{".:53", []string{"."}},
		{"example.org:53", []string{"example.org."}},
		{"example.org.:53", []string{"example.org."}},
		{"10.0.0.0/8:53", []string{"10.in-addr.arpa."}},
		{"10.0.0.0/9", normalizedNonMod8Cidr},
		{"dns://example.org", []string{"example.org."}},
	} {
		actual := Host(test.input).Normalize()
		if len(actual) != len(test.expected) {
			t.Errorf("Test %d: Expected %d elements but %d observed", i, len(test.expected), len(actual))
		} else {
			for ih, host := range actual {
				if host != test.expected[ih] {
					t.Errorf("Test %d: Expected %s at index %d but got %s", i, test.expected[ih], ih, host)
				}
			}
		}
	}
}

func TestSplitHostPortReverse(t *testing.T) {
	tests := map[string]int{
		"example.org.": 0,
		"10.0.0.0/9":   32 - 9,
		"10.0.0.0/8":   32 - 8,
		"10.0.0.0/17":  32 - 17,
		"10.0.0.0/0":   32 - 0,
		"10.0.0.0/64":  0,
		"10.0.0.0":     0,
		"10.0.0":       0,
		"2003::1/65":   128 - 65,
	}
	for in, expect := range tests {
		_, _, n, err := SplitHostPort(in)
		if err != nil {
			t.Errorf("Expected no error, got %q for %s", in, err)
		}
		if n == nil {
			continue
		}
		ones, bits := n.Mask.Size()
		got := bits - ones
		if got != expect {
			t.Errorf("Expected %d, got %d for %s", expect, got, in)
		}
	}
}
