package kubernetes

import (
	"testing"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestParseRequest(t *testing.T) {
	tests := []struct {
		query        string
		expected     string // output from r.String()
		multicluster bool
	}{
		// valid SRV request
		{"_http._tcp.webs.mynamespace.svc.inter.webs.tests.", "http.tcp...webs.mynamespace.svc", false},
		// A request of endpoint
		{"1-2-3-4.webs.mynamespace.svc.inter.webs.tests.", "..1-2-3-4..webs.mynamespace.svc", false},
		// bare zone
		{"inter.webs.tests.", "......", false},
		// bare svc type
		{"svc.inter.webs.tests.", "......", false},
		// bare pod type
		{"pod.inter.webs.tests.", "......", false},
		// SRV request with empty segments
		{"..webs.mynamespace.svc.inter.webs.tests.", "....webs.mynamespace.svc", false},
		// A multicluster request with a clusterid
		{"1-2-3-4.cluster1.webs.mynamespace.svc.inter.webs.tests.", "..1-2-3-4.cluster1.webs.mynamespace.svc", true},
		// An escaped dot is part of the label, not a separator between two labels
		{`foo\.bar.mynamespace.svc.inter.webs.tests.`, `....foo\.bar.mynamespace.svc`, false},
		{`_http._tcp.foo\.bar.mynamespace.svc.inter.webs.tests.`, `http.tcp...foo\.bar.mynamespace.svc`, false},
	}
	for i, tc := range tests {
		m := new(dns.Msg)
		m.SetQuestion(tc.query, dns.TypeA)
		state := request.Request{Zone: zone, Req: m}

		r, e := parseRequest(state.Name(), state.Zone, tc.multicluster)
		if e != nil {
			t.Errorf("Test %d, expected no error, got '%v'.", i, e)
		}
		rs := r.String()
		if rs != tc.expected {
			t.Errorf("Test %d, expected (stringified) recordRequest: %s, got %s", i, tc.expected, rs)
		}
	}
}

func TestParseInvalidRequest(t *testing.T) {
	invalid := []string{
		"webs.mynamespace.pood.inter.webs.test.",                 // Request must be for pod or svc subdomain.
		"too.long.for.what.I.am.trying.to.pod.inter.webs.tests.", // Too long.
	}

	for i, query := range invalid {
		m := new(dns.Msg)
		m.SetQuestion(query, dns.TypeA)
		state := request.Request{Zone: zone, Req: m}

		if _, e := parseRequest(state.Name(), state.Zone, false); e == nil {
			t.Errorf("Test %d: expected error from %s, got none", i, query)
		}
	}
}

const zone = "inter.webs.tests."

func BenchmarkParseRequest(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = parseRequest("1-2-3-4.webs.mynamespace.svc.inter.webs.tests.", zone, false)
	}
}

func TestSplitReverse(t *testing.T) {
	tests := []string{
		"webs.mynamespace.svc",
		"_http._tcp.webs.mynamespace.svc",
		`foo\.bar.mynamespace.svc`,
		`foo\.bar.baz\.qux.svc`,
		`a\\.mynamespace.svc`,
		"..webs.mynamespace.svc",
		".mynamespace.svc",
		"svc",
		"",
	}
	for _, base := range tests {
		var arr [maxSegs]string
		segs, ok := splitReverse(base, &arr)
		if !ok {
			t.Errorf("splitReverse(%q) rejected a name of %d labels", base, len(dns.SplitDomainName(base)))
			continue
		}
		want := dns.SplitDomainName(base)
		if !equalReversed(segs, want) {
			t.Errorf("splitReverse(%q) = %q, want the reverse of %q", base, segs, want)
		}
	}

	// Longer than parseRequest can use.
	var arr [maxSegs]string
	if _, ok := splitReverse("a.b.c.d.e.f.g.svc", &arr); ok {
		t.Error("splitReverse accepted a name with more than maxSegs labels")
	}
}

// FuzzSplitReverse checks the allocation-free split against dns.SplitDomainName,
// which is the splitter the rest of CoreDNS uses and the one this replaces.
func FuzzSplitReverse(f *testing.F) {
	for _, s := range []string{
		"webs.mynamespace.svc",
		`foo\.bar.mynamespace.svc`,
		"..webs.mynamespace.svc",
		"_http._tcp.webs.mynamespace.svc",
		"a.b.c.d.e.f.g.svc",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, base string) {
		if _, ok := dns.IsDomainName(base); !ok {
			t.Skip()
		}

		var arr [maxSegs]string
		segs, ok := splitReverse(base, &arr)
		want := dns.SplitDomainName(base)

		if len(want) > maxSegs {
			if ok {
				t.Fatalf("splitReverse(%q) accepted %d labels, want rejected", base, len(want))
			}
			return
		}
		if !ok {
			t.Fatalf("splitReverse(%q) rejected %d labels", base, len(want))
		}
		if !equalReversed(segs, want) {
			t.Fatalf("splitReverse(%q) = %q, want the reverse of %q", base, segs, want)
		}
	})
}

func equalReversed(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[len(want)-1-i] {
			return false
		}
	}
	return true
}
