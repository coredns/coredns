package file

import (
	"strconv"
	"strings"
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

// Benchmark: measure performance across representative inputs and overshoot cases.
func BenchmarkNameFromRight(b *testing.B) {
	origin := "example.org."
	a := &Zone{origin: origin, origLen: dns.CountLabel(origin)}

	cases := []struct {
		name  string
		qname string
		i     int
	}{
		{"i0_origin", origin, 0},
		{"eq_origin_i1_shot", origin, 1},
		{"two_labels_i1", "a.b." + origin, 1},
		{"two_labels_i2", "a.b." + origin, 2},
		{"two_labels_i3_shot", "a.b." + origin, 3},
		{"ten_labels_i5", strings.Repeat("a.", 10) + origin, 5},
		{"ten_labels_i11_shot", strings.Repeat("a.", 10) + origin, 11},
		{"not_subdomain_shot", "other.tld.", 1},
	}

	var sink int
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			qn, i := tc.qname, tc.i
			for b.Loop() {
				s, shot := a.nameFromRight(qn, i)
				sink += len(s)
				if shot {
					sink++
				}
			}
		})
	}
	if sink == 42 { // prevent elimination
		b.Log(sink)
	}
}

// BenchmarkNameFromRightRandomized iterates over a prebuilt pool
// of qnames and i values to emulate mixed workloads.
func BenchmarkNameFromRightRandomized(b *testing.B) {
	origin := "example.org."
	a := &Zone{origin: origin, origLen: dns.CountLabel(origin)}

	const poolSize = 1024
	type pair struct {
		q string
		i int
	}
	pool := make([]pair, 0, poolSize)

	// Build a variety of qnames with 1..8 labels before origin and various i, including overshoot.
	for n := 1; n <= 8; n++ {
		var sb strings.Builder
		for k := range n {
			sb.WriteString("l")
			sb.WriteString(strconv.Itoa(k))
			sb.WriteByte('.')
		}
		sb.WriteString(origin)
		q := sb.String()
		for i := 0; i <= n+2; i++ {
			pool = append(pool, pair{q: q, i: i})
		}
	}
	// Add some non-subdomain and shorter-than-origin cases.
	pool = append(pool, pair{"org.", 1}, pair{"other.tld.", 1})

	// Ensure pool length is power-of-two for fast masking; if not, trim.
	// Here we just rely on modulo since the pool isn't huge.

	b.ReportAllocs()
	var sink int
	for n := range b.N {
		p := pool[n%len(pool)]
		s, shot := a.nameFromRight(p.q, p.i)
		sink += len(s)
		if shot {
			sink++
		}
	}
	if sink == 43 {
		b.Log(sink)
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
