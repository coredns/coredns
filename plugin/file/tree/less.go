package tree

// less returns <0 when a is less than b, 0 when they are equal and >0 when a is larger than b.
//
// Follows DNSSEC canonical ordering (RFC 4034, Section 6.1):
//   - `\DDD` byte is decoded before comparison
//   - Uppercase A-Z letters are treated as if they were lowercase
//   - Absence of octet sorts before zero value octet
//
// Quirks:
//   - Trailing `\` that escapes nothing is ignored
//   - Leading `\` in `\D` and `\DD` is ignored
//   - Non-FQDN names are assumed to be root-terminated
func less(a, b string) int {
	var (
		adot, bdot   int
		aoff, boff   int
		alast, blast = stripTrailingBackslash(a), stripTrailingBackslash(b)
		ac, bc       byte
	)

	if adot, _ = prevDot(a, alast); alast >= 0 && alast == adot {
		alast--
	}
	if bdot, _ = prevDot(b, blast); blast >= 0 && blast == bdot {
		blast--
	}

	//   dot       off
	//    ▼         ▼
	//  my.exampledomain.com.
	//     ▲           ▲
	//   first        last

	for alast >= 0 && blast >= 0 {
		adot, aoff = prevDot(a, alast)
		bdot, boff = prevDot(b, blast)

		for aoff <= alast && boff <= blast {
			ac, aoff = a[aoff], aoff+1
			if ac == '\\' {
				ac, aoff = nextEscapedByte(a, aoff, alast)
			}
			ac = foldCase(ac)

			bc, boff = b[boff], boff+1
			if bc == '\\' {
				bc, boff = nextEscapedByte(b, boff, blast)
			}
			bc = foldCase(bc)

			if ac != bc {
				return int(ac) - int(bc)
			}
		}

		// Shorter label means less.
		if d := (alast - aoff) - (blast - boff); d != 0 {
			return d
		}

		alast = adot - 1
		blast = bdot - 1
	}

	// Fewer labels means less.
	return alast - blast
}

// stripTrailingBackslash removes hanging backslash that escapes nothing.
func stripTrailingBackslash(s string) (last int) {
	last = len(s) - 1
	for last >= 0 && s[last] == '\\' {
		last--
	}
	if (len(s)-last)%2 == 0 { // `...\` vs `...\\`
		return len(s) - 2
	}
	return len(s) - 1
}

// prevDot finds label-separator dot in [0, last].
func prevDot(s string, last int) (dot, first int) {
	for last >= 0 {
		if s[last] != '.' {
			last--
			continue
		}
		off1 := last - 1
		for off1 >= 0 && s[off1] == '\\' {
			off1--
		}
		if (last-off1)%2 != 0 { // `a\.example` vs `a\\.example`
			break
		}
		last = off1
	}
	return last, last + 1
}

// nextByte implements \DDD-aware and escape-aware advancement.
func nextEscapedByte(s string, off, last int) (byte, int) {
	if off+2 <= last {
		d0, d1, d2 := s[off]-'0', s[off+1]-'0', s[off+2]-'0'
		if d0 < 10 && d1 < 10 && d2 < 10 {
			return d0*100 + d1*10 + d2, off + 3
		}
	}
	return s[off], off + 1
}

func foldCase(c byte) byte {
	if c-'A' < 26 {
		c |= 0x20
	}
	return c
}
