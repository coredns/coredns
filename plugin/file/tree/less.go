package tree

import (
	"bytes"
	"strings"

	"github.com/miekg/dns"
)

// less returns <0 when a is less than b, 0 when they are equal and
// >0 when a is larger than b.
// The function orders names in DNSSEC canonical order: RFC 4034s section-6.1
//
// See https://bert-hubert.blogspot.co.uk/2015/10/how-to-do-fast-canonical-ordering-of.html
// for a blog article on this implementation, although here we still go label by label.
//
// The values of a and b are *not* lowercased before the comparison!
func less(a, b string) int {
	aj := len(a)
	bj := len(b)
	for {
		ai, oka := dns.PrevLabel(a[:aj], 1)
		bi, okb := dns.PrevLabel(b[:bj], 1)
		if oka && okb {
			return 0
		}

		ab := normalizeLabel(a[ai:aj])
		bb := normalizeLabel(b[bi:bj])

		res := bytes.Compare(ab, bb)
		if res != 0 {
			return res
		}

		aj, bj = ai, bi
	}
}

func normalizeLabel(label string) []byte {
	// Compare canonical label bytes, not presentation-format escapes.
	b := []byte(strings.ToLower(label))
	lb := 0
	for i := 0; i < len(b); i++ {
		if b[i] != '\\' || i+1 >= len(b) {
			b[lb] = b[i]
			lb++
			continue
		}

		if i+3 < len(b) && isDigit(b[i+1]) && isDigit(b[i+2]) && isDigit(b[i+3]) {
			b[lb] = dddToByte(b[i:])
			i += 3
			lb++
			continue
		}

		b[lb] = b[i+1]
		i++
		lb++
	}
	return b[:lb]
}

func isDigit(b byte) bool     { return b >= '0' && b <= '9' }
func dddToByte(s []byte) byte { return (s[1]-'0')*100 + (s[2]-'0')*10 + (s[3] - '0') }
