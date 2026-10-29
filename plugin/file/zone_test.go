package file

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

const dbSRVCaseFolding = `
home.arpa. 500 IN SOA ns1.home.arpa. root.home.arpa. 1 6048 4000 2419200 6048
home.arpa. 500 IN NS  ns1.home.arpa.
`

func TestSRVCaseFolding(t *testing.T) {
	tests := []struct {
		name  string
		owner string
		qname string
	}{
		{`\DDD space`, `Home\032Media._smb._tcp.home.arpa.`, `home\ media._smb._tcp.home.arpa.`},
		{"utf-8", `ÄPrinter._ipp._tcp.home.arpa.`, `\195\132printer._ipp._tcp.home.arpa.`},
		{`\DDD UTF-8`, `\195\132Printer._ipp._tcp.home.arpa.`, `\195\132printer._ipp._tcp.home.arpa.`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := dbSRVCaseFolding + tc.owner + " 5 IN SRV 0 0 445 sAmBa.hOmE.arpa.\n"
			zone, err := Parse(strings.NewReader(db), "home.arpa.", "stdin", 0)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			fm := File{
				Next: test.ErrorHandler(),
				Zones: Zones{
					Z:     map[string]*Zone{"home.arpa.": zone},
					Names: []string{"home.arpa."},
				},
			}

			tcase := test.Case{
				Qname: tc.qname, Qtype: dns.TypeSRV,
				Answer: []dns.RR{
					test.SRV(tc.qname + " 5 IN SRV 0 0 445 samba.home.arpa."),
				},
				Ns: []dns.RR{
					test.NS(`home.arpa. 500 IN NS ns1.home.arpa.`),
				},
			}

			rec := dnstest.NewRecorder(&test.ResponseWriter{})
			if _, err := fm.ServeDNS(context.TODO(), rec, tcase.Msg()); err != nil {
				t.Fatalf("ServeDNS: %v", err)
			}

			buf, err := rec.Msg.Pack()
			if err != nil {
				t.Fatalf("Pack: %v", err)
			}
			resp := new(dns.Msg)
			if err := resp.Unpack(buf); err != nil {
				t.Fatalf("Unpack: %v", err)
			}

			if err := test.SortAndCheck(resp, tcase); err != nil {
				t.Error(err)
			}
		})
	}
}

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
