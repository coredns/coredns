package file

import "testing"

func TestNameFromRight(t *testing.T) {
	z := NewZone("example.org.", "stdin")

	tests := []struct {
		in       string
		labels   int
		shot     bool
		expected string
	}{
		{"example.org.", 0, false, "example.org."},
		{"a.example.org.", 0, false, "example.org."},
		{"a.example.org.", 1, false, "a.example.org."},
		{"a.example.org.", 2, true, "a.example.org."},
		{"a.b.example.org.", 2, false, "a.b.example.org."},
	}

	for i, tc := range tests {
		got, shot := z.nameFromRight(tc.in, tc.labels)
		if got != tc.expected {
			t.Errorf("Test %d: expected %s, got %s\n", i, tc.expected, got)
		}
		if shot != tc.shot {
			t.Errorf("Test %d: expected shot to be %t, got %t\n", i, tc.shot, shot)
		}
	}
}

func TestIsNonTerminal(t *testing.T) {
	z := NewZone("example.org.", "stdin")

	tests := []struct {
		in       string
		terminal bool
		expected []string
	}{
		{"b.example.org.", false, nil},
		{"a.b.example.org.", true, []string{"b.example.org."}},
		{"a.b.c.example.org.", true, []string{"b.c.example.org.", "c.example.org."}},
	}

	for i, tc := range tests {
		got, terminal := z.isNonTerminal(tc.in)
		for j, g := range got {
			if g != tc.expected[j] {
				t.Errorf("Test %d: expected %s, got %s\n", i, tc.expected[j], g)
			}
		}
		if terminal != tc.terminal {
			t.Errorf("Test %d: expected terminal to be %t, got %t\n", i, tc.terminal, terminal)
		}
	}
}
