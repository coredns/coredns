package cache

import (
	"encoding/binary"
	"hash/fnv"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

// hashFNV is the reference implementation of hash, using hash/fnv. The inline
// implementation must produce identical keys.
func hashFNV(qname string, qtype, qclass uint16, do, cd bool) uint64 {
	h := fnv.New64()
	if do {
		h.Write([]byte("1"))
	} else {
		h.Write([]byte("0"))
	}
	if cd {
		h.Write([]byte("1"))
	} else {
		h.Write([]byte("0"))
	}
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], qtype)
	h.Write(b[:])
	binary.BigEndian.PutUint16(b[:], qclass)
	h.Write(b[:])
	h.Write([]byte(qname))
	return h.Sum64()
}

func TestHashMatchesFNV64(t *testing.T) {
	names := []string{
		"",
		".",
		"example.org.",
		"EXAMPLE.org.",
		"a.very.long.subdomain.name.that.goes.on.for.a.while.example.com.",
		strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + ".",
		"\x00\xff\x80binary.",
	}
	types := []uint16{0, dns.TypeA, dns.TypeAAAA, dns.TypeMX, dns.TypeHTTPS, dns.TypeANY, 0xff00, 0xffff}
	classes := []uint16{0, dns.ClassINET, dns.ClassCHAOS, dns.ClassANY, 0xffff}
	bools := []bool{false, true}

	for _, name := range names {
		for _, qtype := range types {
			for _, qclass := range classes {
				for _, do := range bools {
					for _, cd := range bools {
						got := hash(name, qtype, qclass, do, cd)
						want := hashFNV(name, qtype, qclass, do, cd)
						if got != want {
							t.Errorf("hash(%q, %d, %d, %t, %t) = %#x, want %#x", name, qtype, qclass, do, cd, got, want)
						}
					}
				}
			}
		}
	}
}

// hashSink keeps the result live so the compiler cannot drop the call.
var hashSink uint64

func BenchmarkHash(b *testing.B) {
	for _, bc := range []struct {
		name  string
		qname string
	}{
		{"short", "example.org."},
		{"long", "a.very.long.subdomain.name.that.goes.on.for.a.while.example.com."},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				hashSink = hash(bc.qname, dns.TypeA, dns.ClassINET, true, false)
			}
		})
	}
}
