package plugin

import (
	"bytes"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/miekg/dns"
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

func TestNameCompare(t *testing.T) {
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

	less := func(a, b string) int {
		return Name(a).Compare(b)
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

func TestNameCompareRelation_EdgeCases(t *testing.T) {
	// For every case four variants are synthesized:
	//  - a  b
	//  - a. b
	//  - a  b.
	//  - a. b.
	//
	// For each variant commutativity is tested.
	tests := []struct {
		a, b           string
		variants       bool
		wantCmp        int
		wantNested     bool
		wantAI, wantBI int
	}{
		{``, ``, true, 0, true, 0, 0},
		{``, `\000`, true, -1, true, 0, 4},
		{``, `\.`, true, -1, true, 0, 2},
		{`\.`, `\.`, true, 0, true, 0, 0},
		{``, `example`, true, -1, true, 0, 7},
		{`example`, `example`, true, 0, true, 0, 0},
		{`a\.example`, `a.example`, true, -1, false, 10, 9},
		{`a.example`, `a-b.example`, true, -1, false, 2, 4},
		{`a.example`, `a*.example`, true, -1, false, 2, 3},
		{`a.example`, `a\000.example`, true, -1, false, 2, 6},
		{`a.eXaMpLe`, `a.example`, true, 0, true, 0, 0},
		{`\000\0320 \"\046@*`, `\000\032\048\032\034\046\064\042`, true, 0, true, 0, 0},
		{`<=>?@ABCDE`, `\060\061\062\063\064\065\066\067\068\069`, true, 0, true, 0, 0},
		{`<=>?@ABCDE`, `\060\061\062\063\064\097\098\099\100\101`, true, 0, true, 0, 0},
		{`café.example`, `CAFÉ.example`, true, 1, false, 6, 6}, // é (\195\169) > É (\195\137)
		{`\\065.example`, `\\097.example`, true, -1, false, 6, 6},
		{``, `\`, false, 0, true, 0, 0},
		{`a\.b.example`, `a\046b.example`, true, 0, true, 0, 0},
		{`0.example`, `\0.example`, true, 0, true, 0, 0},
		{`01.example`, `\01.example`, true, 0, true, 0, 0},
		{`a\.b.example`, `a\046b.example`, true, 0, true, 0, 0},
		{`\\.example`, `\092.example`, true, 0, true, 0, 0},
		{`a.B.example`, `b.EXAMPLE`, true, 1, true, 2, 0},
		{`\097.b.example`, `B.EXAMPLE`, true, 1, true, 5, 0},
		{`\..example`, `example`, true, 1, true, 3, 0},
		{`a\\.example`, `example`, true, 1, true, 4, 0},
		{`aexample`, `example`, true, -1, false, 8, 7},
		{`a\.example`, `example`, true, -1, false, 10, 7},
		{`a\\\.example`, `example`, true, -1, false, 12, 7},
		{`\046example`, `example`, true, -1, false, 11, 7},
		{`example`, `example\.`, true, -1, false, 7, 9},
		{`x.a\.b.example`, `a.b.example`, true, -1, false, 7, 4},
	}
	for i, test := range tests {
		variants := []struct{ a, b string }{
			{test.a, test.b},
			{test.a + `.`, test.b},
			{test.a, test.b + `.`},
			{test.a + `.`, test.b + `.`},
		}
		if !test.variants {
			variants = variants[:1]
		}

		for _, variant := range variants {
			cmp, nested, ni := Name(variant.a).CompareRelation(variant.b)
			cmp = max(-1, min(1, cmp))
			if cmp != test.wantCmp || nested != test.wantNested || ni != test.wantAI {
				t.Errorf("Test %d: expected CompareRelation(%s, %s)=(%v, %v, %v), got (%v, %v, %v)",
					i, variant.a, variant.b, test.wantCmp, test.wantNested, test.wantAI, cmp, nested, ni)
			}

			cmp, nested, ni = Name(variant.b).CompareRelation(variant.a)
			cmp = max(-1, min(1, cmp))
			if cmp != -test.wantCmp || nested != test.wantNested || ni != test.wantBI {
				t.Errorf("Test %d: expected CompareRelation(%s, %s)=(%v, %v, %v), got (%v, %v, %v)",
					i, variant.b, variant.a, -test.wantCmp, test.wantNested, test.wantBI, cmp, nested, ni)
			}
		}
	}
}

func BenchmarkNameCompare(b *testing.B) {
	less := func(a, b string) int {
		return Name(a).Compare(b)
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

// The original less function, serving as the benchmark test baseline.
func less0(a, b string) int {
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
