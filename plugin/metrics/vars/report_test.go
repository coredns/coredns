package vars

import (
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestReportWithOptions(t *testing.T) {
	tests := []struct {
		name         string
		question     string
		qtype        uint16
		edns0        bool
		do           bool
		originalSize int
		useOriginal  bool
	}{
		{
			name:         "A record without DO bit",
			question:     "example.org.",
			qtype:        dns.TypeA,
			edns0:        true,
			do:           false,
			originalSize: 0,
			useOriginal:  false,
		},
		{
			name:         "A record with DO bit",
			question:     "example.org.",
			qtype:        dns.TypeA,
			edns0:        true,
			do:           true,
			originalSize: 0,
			useOriginal:  false,
		},
		{
			name:         "A record with original size",
			question:     "example.org.",
			qtype:        dns.TypeA,
			edns0:        false,
			do:           false,
			originalSize: 42,
			useOriginal:  true,
		},
		{
			name:         "A record bogus qtype",
			question:     "example.org.",
			qtype:        0, // does not exist
			edns0:        false,
			do:           false,
			originalSize: 42,
			useOriginal:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion(tc.question, tc.qtype)
			if tc.edns0 {
				m.SetEdns0(4096, tc.do)
			}

			w := &test.ResponseWriter{}
			state := request.Request{W: w, Req: m}

			if state.Do() != tc.do {
				t.Errorf("DO bit detection failed, got %v, want %v", state.Do(), tc.do)
			}

			qType := qTypeString(tc.qtype)
			expectedType := dns.Type(tc.qtype).String()
			if qType != expectedType && qType != "other" {
				t.Errorf("qTypeString(%d) = %s, want %s or 'other'", tc.qtype, qType, expectedType)
			}

			var opts []ReportOption
			if tc.useOriginal {
				opts = append(opts, WithOriginalReqSize(tc.originalSize))
			}

			net := state.Proto()
			fam := "1"

			countBefore := testutil.ToFloat64(RequestCount.WithLabelValues("dns://:53", "example.org.", "", net, fam, qType))

			Report("dns://:53", state, "example.org.", "", "NOERROR", "test", 100, time.Now(), opts...)

			countAfter := testutil.ToFloat64(RequestCount.WithLabelValues("dns://:53", "example.org.", "", net, fam, qType))
			if countAfter <= countBefore {
				t.Errorf("RequestCount was not incremented. Before: %f, After: %f", countBefore, countAfter)
			}
		})
	}
}
