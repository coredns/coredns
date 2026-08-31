package tree

import (
	"github.com/miekg/dns"
)

// less returns <0 when a is less than b, 0 when they are equal and >0 when a is larger than b.
//
// Follows DNSSEC canonical ordering (RFC 4034, Section 6.1):
//   - \DDD byte is decoded before comparison
//   - Uppercase A-Z letters are treated as if they were lowercase
//   - Absence of octet sorts before zero value octet
func less(a, b string) int {
	for {
		ai, _ := dns.PrevLabel(a, 1)
		bi, _ := dns.PrevLabel(b, 1)

		var (
			ac, bc     byte
			aoff, boff = ai, bi
		)
		for aoff < len(a)-1 && boff < len(b)-1 {
			ac, aoff = nextByte(a, aoff)
			if ac-'A' < 26 {
				ac |= 0x20
			}

			bc, boff = nextByte(b, boff)
			if bc-'A' < 26 {
				bc |= 0x20
			}

			if ac != bc {
				return int(ac) - int(bc)
			}
		}

		if d := (len(a) - aoff) - (len(b) - boff); d != 0 {
			return d
		}

		// Exit early when either of strings is out of labels.
		if ai == 0 || bi == 0 {
			return ai - bi
		}

		a, b = a[:ai], b[:bi]
	}
}

// nextByte implements \DDD-aware advancement.
func nextByte(s string, off int) (byte, int) {
	b := s[off]
	if b == '\\' && off+3 < len(s) {
		d0, d1, d2 := s[off+1]-'0', s[off+2]-'0', s[off+3]-'0'
		if d0 < 10 && d1 < 10 && d2 < 10 {
			return d0*100 + d1*10 + d2, off + 4
		}
	}
	return b, off + 1
}
