package plugin

import (
	"strconv"
	"strings"
	"testing"
)

var zoneMatchSink string

func BenchmarkZoneMatches(b *testing.B) {
	zones := Zones{
		"org.",
		"net.",
		"example.com.",
		"example.net.",
		"other.example.",
		"cluster.local.",
		"example.org.",
		".",
	}
	for _, n := range []int{4, 32} {
		qname := benchQName(n)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				zoneMatchSink = zones.Matches(qname)
			}
		})
	}
}

func benchQName(extra int) string {
	var b strings.Builder
	for range extra {
		b.WriteString("abc.")
	}
	b.WriteString("example.org.")
	return b.String()
}
