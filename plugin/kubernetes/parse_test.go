package kubernetes

import (
	"slices"
	"testing"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestParseRequest(t *testing.T) {
	tests := []struct {
		query        string
		expected     string // output from r.String()
		multicluster bool
		zonal        bool
		zone         string // expected r.zone
	}{
		// valid SRV request
		{"_http._tcp.webs.mynamespace.svc.inter.webs.tests.", "http.tcp...webs.mynamespace.svc", false, false, ""},
		// A request of endpoint
		{"1-2-3-4.webs.mynamespace.svc.inter.webs.tests.", "..1-2-3-4..webs.mynamespace.svc", false, false, ""},
		// bare zone
		{"inter.webs.tests.", "......", false, false, ""},
		// bare svc type
		{"svc.inter.webs.tests.", "......", false, false, ""},
		// bare pod type
		{"pod.inter.webs.tests.", "......", false, false, ""},
		// SRV request with empty segments
		{"..webs.mynamespace.svc.inter.webs.tests.", "....webs.mynamespace.svc", false, false, ""},
		// A multicluster request with a clusterid
		{"1-2-3-4.cluster1.webs.mynamespace.svc.inter.webs.tests.", "..1-2-3-4.cluster1.webs.mynamespace.svc", true, false, ""},
		// An escaped dot is part of the label, not a separator between two labels
		{`foo\.bar.mynamespace.svc.inter.webs.tests.`, `....foo\.bar.mynamespace.svc`, false, false, ""},
		{`_http._tcp.foo\.bar.mynamespace.svc.inter.webs.tests.`, `http.tcp...foo\.bar.mynamespace.svc`, false, false, ""},
		// zone-scoped names, both directives
		{"us-west-2a.pin._zone.webs.mynamespace.svc.inter.webs.tests.", "....webs.mynamespace.svc", false, true, "us-west-2a"},
		{"us-west-2a.prefer._zone.webs.mynamespace.svc.inter.webs.tests.", "....webs.mynamespace.svc", false, true, "us-west-2a"},
		// zone label values may contain dots; the value spans every label
		// left of the directive
		{"corp.example.com.pin._zone.webs.mynamespace.svc.inter.webs.tests.", "....webs.mynamespace.svc", false, true, "corp.example.com"},
		// two-labels-left with an underscore still reads as port/protocol
		{"us-west-2a._zone.webs.mynamespace.svc.inter.webs.tests.", "us-west-2a.zone...webs.mynamespace.svc", false, true, ""},
	}
	for i, tc := range tests {
		m := new(dns.Msg)
		m.SetQuestion(tc.query, dns.TypeA)
		state := request.Request{Zone: zone, Req: m}

		r, e := parseRequest(state.Name(), state.Zone, tc.multicluster, tc.zonal)
		if e != nil {
			t.Errorf("Test %d, expected no error, got '%v'.", i, e)
		}
		rs := r.String()
		if rs != tc.expected {
			t.Errorf("Test %d, expected (stringified) recordRequest: %s, got %s", i, tc.expected, rs)
		}
		if r.zone != tc.zone {
			t.Errorf("Test %d, expected zone %q, got %q", i, tc.zone, r.zone)
		}
	}
}

func TestParseInvalidRequest(t *testing.T) {
	invalid := []string{
		"webs.mynamespace.pood.inter.webs.test.",                      // Request must be for pod or svc subdomain.
		"too.long.for.what.I.am.trying.to.pod.inter.webs.tests.",      // Too long.
		"us-west-2a.pin._zone.webs.mynamespace.svc.inter.webs.tests.", // Zonal shape without the zonal option.
	}

	// The zonal-shaped rejections that must hold even WITH the option on:
	// wrong subtree, unknown directive, and multicluster zones.
	zonalInvalid := []struct {
		query        string
		multicluster bool
	}{
		{"us-west-2a.pin._zone.webs.mynamespace.pod.inter.webs.tests.", false},
		{"us-west-2a.florp._zone.webs.mynamespace.svc.inter.webs.tests.", false},
		{"us-west-2a.pin._zone.webs.mynamespace.svc.inter.webs.tests.", true},
	}
	for i, tc := range zonalInvalid {
		m := new(dns.Msg)
		m.SetQuestion(tc.query, dns.TypeA)
		state := request.Request{Zone: zone, Req: m}
		if _, e := parseRequest(state.Name(), state.Zone, tc.multicluster, true); e == nil {
			t.Errorf("Zonal-invalid test %d: expected error from %s, got none", i, tc.query)
		}
	}

	for i, query := range invalid {
		m := new(dns.Msg)
		m.SetQuestion(query, dns.TypeA)
		state := request.Request{Zone: zone, Req: m}

		if _, e := parseRequest(state.Name(), state.Zone, false, false); e == nil {
			t.Errorf("Test %d: expected error from %s, got none", i, query)
		}
	}
}

const zone = "inter.webs.tests."

func BenchmarkParseRequest(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = parseRequest("1-2-3-4.webs.mynamespace.svc.inter.webs.tests.", zone, false, false)
	}
}

// FuzzSplitDomainName checks the allocation-free split against dns.SplitDomainName.
func FuzzSplitDomainName(f *testing.F) {
	for _, s := range []string{
		"webs.mynamespace.svc",
		"_http._tcp.webs.mynamespace.svc",
		"corp.example.com.pin._zone.webs.mynamespace.svc",
		`foo\.bar.mynamespace.svc`,
		`a\\.mynamespace.svc`,
		"..webs.mynamespace.svc",
		"webs..svc",
		".svc",
		"svc.",
		"",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		var arr [maxLabels]string
		if got, want := splitDomainName(name, &arr), dns.SplitDomainName(name); !slices.Equal(got, want) {
			t.Errorf("splitDomainName(%q) = %q, want %q", name, got, want)
		}
	})
}
