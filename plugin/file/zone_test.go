package file

import (
	"testing"

	"github.com/coredns/coredns/plugin/file/tree"

	"github.com/miekg/dns"
)

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
			t.Errorf("Test %d: expected %s, got %s", i, tc.expected, got)
		}
		if shot != tc.shot {
			t.Errorf("Test %d: expected shot to be %t, got %t", i, tc.shot, shot)
		}
	}
}

func TestInsertPreservesSRVCase(t *testing.T) {
	z := NewZone("home.arpa.", "stdin")

	// SRV with mixed case and space-escaped instance name
	srv, err := dns.NewRR(`Home\032Media._smb._tcp.home.arpa. 5 IN SRV 0 0 445 samba.home.arpa.`)
	if err != nil {
		t.Fatalf("Failed to parse SRV RR: %v", err)
	}

	if err := z.Insert(srv); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	found := false
	err = z.Walk(func(elem *tree.Elem, rrsets map[uint16][]dns.RR) error {
		for _, rrs := range rrsets {
			for _, rr := range rrs {
				if srvRR, ok := rr.(*dns.SRV); ok {
					if srvRR.Hdr.Name == "Home\\032Media._smb._tcp.home.arpa." {
						found = true
						if srvRR.Target != "samba.home.arpa." {
							t.Errorf("Expected SRV target to be 'samba.home.arpa.', got %q", srvRR.Target)
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Tree walk failed: %v", err)
	}

	if !found {
		t.Errorf("SRV record with original case not found in tree")
	}
}
