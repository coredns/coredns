package hosts

import (
	"testing"
)

func TestReplaceWithAsteriskLabel(t *testing.T) {
	tests := []struct {
		in, out string
	}{
		{".", ""},
		{"example.com.", "*.com."},
		{"foo.example.com.", "*.example.com."},
		{"bar.intern.example.com.", "*.intern.example.com."},
	}

	for _, tc := range tests {
		got := replaceWithAsteriskLabel(tc.in)
		if got != tc.out {
			t.Errorf("replaceWithAsteriskLabel(%q) = %q, want %q", tc.in, got, tc.out)
		}
	}
}

func TestReplaceWithAsteriskLabelApex(t *testing.T) {
	// Apex names produce a *.parent pattern but must not match *.zone wildcards.
	if got := replaceWithAsteriskLabel("example.com."); got != "*.com." {
		t.Fatalf("replaceWithAsteriskLabel(example.com.) = %q, want *.com.", got)
	}
}

func TestLookupWildcardHost(t *testing.T) {
	const hosts = `
127.0.0.53 *.example.org
127.0.1.52 *.intern.example.org
127.0.0.54 foo.example.org
192.168.33.10 *.example.com
192.168.33.11 a.example.com
192.168.33.12 b.example.com
`

	h := testHostsfile(hosts)

	tests := []staticHostEntry{
		{"foo.example.org.", []string{"127.0.0.54"}, []string{}},
		{"bar.example.org.", []string{"127.0.0.53"}, []string{}},
		{"bar.intern.example.org.", []string{"127.0.1.52"}, []string{}},
		{"a.example.com.", []string{"192.168.33.11"}, []string{}},
		{"b.example.com.", []string{"192.168.33.12"}, []string{}},
		{"c.example.com.", []string{"192.168.33.10"}, []string{}},
		{"example.com.", []string{}, []string{}},
		{"deep.foo.example.org.", []string{}, []string{}},
	}

	for _, ent := range tests {
		testStaticHost(t, ent, h)
	}
}
