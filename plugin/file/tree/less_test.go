package tree

import (
	"bytes"
	"cmp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/miekg/dns"
)

func TestLess(t *testing.T) {
	tests := []struct {
		in  []string
		out []string
	}{
		{
			[]string{"aaa.powerdns.de.", "bbb.powerdns.net.", "xxx.powerdns.com."},
			[]string{"xxx.powerdns.com.", "aaa.powerdns.de.", "bbb.powerdns.net."},
		},
		{
			[]string{"aaa.POWERDNS.de.", "bbb.PoweRdnS.net.", "xxx.powerdns.com."},
			[]string{"xxx.powerdns.com.", "aaa.POWERDNS.de.", "bbb.PoweRdnS.net."},
		},
		{
			[]string{"aaa.aaaa.aa.", "aa.aaa.a.", "bbb.bbbb.bb."},
			[]string{"aa.aaa.a.", "aaa.aaaa.aa.", "bbb.bbbb.bb."},
		},
		{
			[]string{"aaaaa.", "aaa.", "bbb."},
			[]string{"aaa.", "aaaaa.", "bbb."},
		},
		{
			[]string{"a.a.a.a.", "a.a.", "a.a.a."},
			[]string{"a.a.", "a.a.a.", "a.a.a.a."},
		},
		{
			[]string{"example.", "z.example.", "a.example."},
			[]string{"example.", "a.example.", "z.example."},
		},
		{
			[]string{"a.example.", "Z.a.example.", "z.example.", "yljkjljk.a.example.", "\\001.z.example.", "example.", "*.z.example.", "\\200.z.example.", "zABC.a.EXAMPLE."},
			[]string{"example.", "a.example.", "yljkjljk.a.example.", "Z.a.example.", "zABC.a.EXAMPLE.", "z.example.", "\\001.z.example.", "*.z.example.", "\\200.z.example."},
		},
		{
			// RFC3034 example.
			[]string{"a.example.", "Z.a.example.", "z.example.", "yljkjljk.a.example.", "example.", "*.z.example.", "zABC.a.EXAMPLE."},
			[]string{"example.", "a.example.", "yljkjljk.a.example.", "Z.a.example.", "zABC.a.EXAMPLE.", "z.example.", "*.z.example."},
		},
	}

Tests:
	for j, test := range tests {
		slices.SortFunc(test.in, less)
		for i := range len(test.in) {
			if test.in[i] != test.out[i] {
				t.Errorf("Test %d: expected %s, got %s", j, test.out[i], test.in[i])
				n := ""
				for k, in := range test.in {
					if k+1 == len(test.in) {
						n = "\n"
					}
					t.Logf("%s <-> %s\n%s", in, test.out[k], n)
				}
				continue Tests
			}
		}
	}
}

// Test that concurrent calls to Less (which calls Elem.Name) do not race or panic.
// See issue #7561 for reference.
func TestLess_ConcurrentNameAccess(t *testing.T) {
	rr, err := dns.NewRR("a.example. 3600 IN A 1.2.3.4")
	if err != nil {
		t.Fatalf("failed to create RR: %v", err)
	}
	e := newElem(rr)

	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			// Compare the same name repeatedly; previously this could race due to lazy Name() writes.
			_ = Less(e, "a.example.")
			_ = e.Name()
		}()
	}
	wg.Wait()
}

func TestLess_EdgeCases(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		want int
	}{
		{``, ``, 0},
		{``, `\000`, -1},
		{``, `example.`, -1},
		{`a.example.`, `a-b.example.`, -1},
		{`a.example.`, `a*.example.`, -1},
		{`a.example.`, `a\000.example.`, -1},
		{`a.eXaMpLe.`, `a.example.`, 0},
		{`.`, `\000.`, -1},
		{`\000\0320 \"\046@*.`, `\000\032\048\032\092\034\046\064\042.`, 0},
		{`<=>?@ABCDE.`, `\060\061\062\063\064\065\066\067\068\069.`, 0},
		{`café.example.`, `CAFÉ.example.`, 1}, // é (\195\169) > É (\195\137)
	}
	for i, test := range tests {
		if got := less(test.a, test.b); cmp.Compare(got, 0) != test.want {
			t.Errorf("Test %d: expected less(%s, %s)=%d, got %d", i, test.a, test.b, test.want, cmp.Compare(got, 0))
		}

		if got := less(test.b, test.a); cmp.Compare(got, 0) != -test.want {
			t.Errorf("Test %d: expected less(%s, %s)=%d, got %d", i, test.b, test.a, -test.want, cmp.Compare(got, 0))
		}
	}
}

func BenchmarkLess(b *testing.B) {
	// The original less function, serving as the benchmark test baseline.
	less0 := func(a, b string) int {
		i := 1
		aj := len(a)
		bj := len(b)
		for {
			ai, oka := dns.PrevLabel(a, i)
			bi, okb := dns.PrevLabel(b, i)
			if oka && okb {
				return 0
			}

			// sadly this []byte will allocate... TODO(miek): check if this is needed
			// for a name, otherwise compare the strings.
			ab := []byte(strings.ToLower(a[ai:aj]))
			bb := []byte(strings.ToLower(b[bi:bj]))
			doDDD(ab)
			doDDD(bb)

			res := bytes.Compare(ab, bb)
			if res != 0 {
				return res
			}

			i++
			aj, bj = ai, bi
		}
	}

	tests := [][]string{
		{"aaa.powerdns.de", "bbb.powerdns.net.", "xxx.powerdns.com."},
		{"aaa.POWERDNS.de", "bbb.PoweRdnS.net.", "xxx.powerdns.com."},
		{"aaa.aaaa.aa.", "aa.aaa.a.", "bbb.bbbb.bb."},
		{"aaaaa.", "aaa.", "bbb."},
		{"a.a.a.a.", "a.a.", "a.a.a."},
		{"example.", "z.example.", "a.example."},
		{"a.example.", "Z.a.example.", "z.example.", "yljkjljk.a.example.", "\\001.z.example.", "example.", "*.z.example.", "\\200.z.example.", "zABC.a.EXAMPLE."},
		{"a.example.", "Z.a.example.", "z.example.", "yljkjljk.a.example.", "example.", "*.z.example.", "zABC.a.EXAMPLE."},
	}
	b.ResetTimer()

	b.Run("base", func(b *testing.B) {
		for b.Loop() {
			for _, t := range tests {
				for m := range len(t) - 1 {
					for n := m + 1; n < len(t); n++ {
						less0(t[m], t[n])
					}
				}
			}
		}
	})

	b.Run("optimized", func(b *testing.B) {
		for b.Loop() {
			for _, t := range tests {
				for m := range len(t) - 1 {
					for n := m + 1; n < len(t); n++ {
						less(t[m], t[n])
					}
				}
			}
		}
	})
}

func doDDD(b []byte) {
	lb := len(b)
	for i := 0; i < lb; i++ {
		if i+3 < lb && b[i] == '\\' && isDigit(b[i+1]) && isDigit(b[i+2]) && isDigit(b[i+3]) {
			b[i] = dddToByte(b[i:])
			for j := i + 1; j < lb-3; j++ {
				b[j] = b[j+3]
			}
			lb -= 3
		}
	}
}
func isDigit(b byte) bool     { return b >= '0' && b <= '9' }
func dddToByte(s []byte) byte { return (s[1]-'0')*100 + (s[2]-'0')*10 + (s[3] - '0') }
