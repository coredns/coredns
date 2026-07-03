package dnsutil

import "testing"

func TestJoin(t *testing.T) {
	tests := []struct {
		in  []string
		out string
	}{
		{[]string{"bla", "bliep", "example", "org"}, "bla.bliep.example.org."},
		{[]string{"example", "."}, "example."},
		{[]string{"example", "org."}, "example.org."}, // technically we should not be called like this.
		{[]string{"."}, "."},
	}

	for i, tc := range tests {
		if x := Join(tc.in...); x != tc.out {
			t.Errorf("Test %d, expected %s, got %s", i, tc.out, x)
		}
	}
}

func TestJoinEmpty(t *testing.T) {
	// Join called with no labels must not index labels[ll-1] out of range; it
	// returns the root name. Callers on the MX/SRV path can reach Join with an
	// empty label slice.
	if x := Join(); x != "." {
		t.Errorf("expected %q, got %q", ".", x)
	}
}
