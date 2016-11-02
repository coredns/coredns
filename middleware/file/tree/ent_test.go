package tree

import (
	"testing"

	"github.com/miekg/dns"
)

func TestIsNonTerminal(t *testing.T) {
	tr := Tree{}
	tr.OrigLen = 2

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
		d, _ := dns.NewRR(tc.in + " IN A 127.0.0.1")
		got, terminal := tr.isNonTerminal(d)
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
