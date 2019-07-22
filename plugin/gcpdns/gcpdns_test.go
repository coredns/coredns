package gcpdns

import (
	"context"
	"reflect"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/plugin/test"
	crequest "github.com/coredns/coredns/request"
	dnslib "github.com/miekg/dns"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/googleapi"
)

func TestConfiguredWithNonexistentZone(t *testing.T) {
	ctx := context.Background()

	_, err := newGcpDNSHandler(ctx, newMock(), map[string][]zoneID{"bad.": {zoneID{"my-project", "unknown-zone"}}}, &upstream.Upstream{})
	assert.Error(t, err)
	assert.Equal(t, 404, errors.Cause(err).(*googleapi.Error).Code)
}

func TestConfiguredWithNonexistentProject(t *testing.T) {
	ctx := context.Background()

	_, err := newGcpDNSHandler(ctx, newMock(), map[string][]zoneID{"bad.": {zoneID{"unknown-project", "org-zone"}}}, &upstream.Upstream{})
	assert.Error(t, err)
	assert.Equal(t, 403, errors.Cause(err).(*googleapi.Error).Code)
}

func TestServeDNS(t *testing.T) {
	ctx := context.Background()

	r, err := newGcpDNSHandler(ctx, newMock(), map[string][]zoneID{
		"org.": {zoneID{"my-project", "another-org-zone"}, zoneID{"my-project", "org-zone"}},
		"gov.": {zoneID{"my-project", "org-zone"},
		}}, &upstream.Upstream{})
	if err != nil {
		t.Fatalf("Failed to create gcpdns: %v", err)
	}
	r.Fall = fall.Zero
	r.Fall.SetZonesFromArgs([]string{"gov."})
	r.Next = test.HandlerFunc(func(ctx context.Context, w dnslib.ResponseWriter, r *dnslib.Msg) (int, error) {
		state := crequest.Request{W: w, Req: r}
		qname := state.Name()
		m := new(dnslib.Msg)
		rcode := dnslib.RcodeServerFailure
		if qname == "example.gov." {
			m.SetReply(r)
			rr, err := dnslib.NewRR("example.gov.  300 IN  A   2.4.6.8")
			if err != nil {
				t.Fatalf("Failed to create Resource Record: %v", err)
			}
			m.Answer = []dnslib.RR{rr}

			m.Authoritative = true
			rcode = dnslib.RcodeSuccess

		}

		m.SetRcode(r, rcode)
		assert.NoError(t, w.WriteMsg(m))
		return rcode, nil
	})
	err = r.startup(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize gcpdns: %v", err)
	}

	tests := []struct {
		name         string
		qname        string
		qtype        uint16
		wantRetCode  int
		wantAnswer   []string // ownernames for the records in the additional section.
		wantMsgRCode int
		wantNS       []string
		expectedErr  error
	}{
		// 0. example.org A found - success.
		{
			name:       "example.org-A",
			qname:      "example.org",
			qtype:      dnslib.TypeA,
			wantAnswer: []string{"example.org.	300	IN	A	1.2.3.4"},
		},
		// 1. example.org AAAA found - success.
		{
			name:       "example.org-AAAA",
			qname:      "example.org",
			qtype:      dnslib.TypeAAAA,
			wantAnswer: []string{"example.org.	300	IN	AAAA	2001:db8:85a3::8a2e:370:7334"},
		},
		// 2. exampled.org PTR found - success.
		{
			name:       "example.org-PTR",
			qname:      "example.org",
			qtype:      dnslib.TypePTR,
			wantAnswer: []string{"example.org.	300	IN	PTR	ptr.example.org."},
		},
		// 3. sample.example.org points to example.org CNAME.
		// Query must return both CNAME and A recs.
		{
			name:  "sample.example.org-A",
			qname: "sample.example.org",
			qtype: dnslib.TypeA,
			wantAnswer: []string{
				"sample.example.org.	300	IN	CNAME	example.org.",
				"example.org.	300	IN	A	1.2.3.4",
			},
		},
		// 4. Explicit CNAME query for sample.example.org.
		// Query must return just CNAME.
		{
			name:       "sample.example.org-CNAME",
			qname:      "sample.example.org",
			qtype:      dnslib.TypeCNAME,
			wantAnswer: []string{"sample.example.org.	300	IN	CNAME	example.org."},
		},
		// 5. Explicit SOA query for example.org.
		{
			name:       "example.org-SOA",
			qname:      "example.org",
			qtype:      dnslib.TypeSOA,
			wantAnswer: []string{"org.	300	IN	SOA	ns-cloud-d1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300"},
		},
		// 6. Explicit SOA query for example.org.
		{
			name:   "example.org-NS",
			qname:  "example.org",
			qtype:  dnslib.TypeNS,
			wantNS: []string{"org.	300	IN	SOA	ns-cloud-a1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300"},
		},
		// 7. AAAA query for split-example.org must return NODATA.
		{
			name:        "split-example.gov-AAAA",
			qname:       "split-example.gov",
			qtype:       dnslib.TypeAAAA,
			wantRetCode: dnslib.RcodeSuccess,
			wantNS:      []string{"org.	300	IN	SOA	ns-cloud-a1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300"},
		},
		// 8. Zone not configured.
		{
			name:         "badexample.com",
			qname:        "badexample.com",
			qtype:        dnslib.TypeA,
			wantRetCode:  dnslib.RcodeServerFailure,
			wantMsgRCode: dnslib.RcodeServerFailure,
		},
		// 9. No record found. Return SOA record.
		{
			name:         "bad.org",
			qname:        "bad.org",
			qtype:        dnslib.TypeA,
			wantRetCode:  dnslib.RcodeSuccess,
			wantMsgRCode: dnslib.RcodeNameError,
			wantNS:       []string{"org.	300	IN	SOA	ns-cloud-a1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300"},
		},
		// 10. No record found. Fallthrough.
		{
			name:       "example.gov",
			qname:      "example.gov",
			qtype:      dnslib.TypeA,
			wantAnswer: []string{"example.gov.	300	IN	A	2.4.6.8"},
		},
		// 11. other-zone.example.org is stored in a different hosted zone. success
		{
			name:       "other-example.org",
			qname:      "other-example.org",
			qtype:      dnslib.TypeA,
			wantAnswer: []string{"other-example.org.	300	IN	A	3.5.7.9"},
		},
		// 12. split-example.org only has A record. Expect NODATA.
		{
			name:   "split-example.org",
			qname:  "split-example.org",
			qtype:  dnslib.TypeAAAA,
			wantNS: []string{"org.	300	IN	SOA	ns-cloud-d1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300"},
		},
		// 13. *.www.example.org is a wildcard CNAME to www.example.org.
		{
			name:  "a.www.example.org",
			qname: "a.www.example.org",
			qtype: dnslib.TypeA,
			wantAnswer: []string{
				"a.www.example.org.	300	IN	CNAME	www.example.org.",
				"www.example.org.	300	IN	A	1.2.3.4",
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			req := new(dnslib.Msg)
			req.SetQuestion(dnslib.Fqdn(tc.qname), tc.qtype)

			rec := dnstest.NewRecorder(&test.ResponseWriter{})
			code, err := r.ServeDNS(ctx, rec, req)

			if err != tc.expectedErr {
				t.Fatalf("Expected error %v, but got %v", tc.expectedErr, err)
			}
			if code != int(tc.wantRetCode) {
				t.Fatalf("Expected returned status code %s, but got %s", dnslib.RcodeToString[tc.wantRetCode], dnslib.RcodeToString[code])
			}

			if tc.wantMsgRCode != rec.Msg.Rcode {
				t.Errorf("Unexpected msg status code. Want: %s, got: %s", dnslib.RcodeToString[tc.wantMsgRCode], dnslib.RcodeToString[rec.Msg.Rcode])
			}

			if len(tc.wantAnswer) != len(rec.Msg.Answer) {
				t.Errorf("Unexpected number of Answers. Want: %d, got: %d", len(tc.wantAnswer), len(rec.Msg.Answer))
			} else {
				for i, gotAnswer := range rec.Msg.Answer {
					if gotAnswer.String() != tc.wantAnswer[i] {
						t.Errorf("Unexpected answer.\nWant:\n\t%s\nGot:\n\t%s", tc.wantAnswer[i], gotAnswer)
					}
				}
			}

			if len(tc.wantNS) != len(rec.Msg.Ns) {
				t.Errorf("Unexpected NS number. Want: %d, got: %d", len(tc.wantNS), len(rec.Msg.Ns))
			} else {
				for i, ns := range rec.Msg.Ns {
					got, ok := ns.(*dnslib.SOA)
					if !ok {
						t.Errorf("Unexpected NS type. Want: SOA, got: %v", reflect.TypeOf(got))
					}
					if got.String() != tc.wantNS[i] {
						t.Errorf("Unexpected NS.\nWant: %v\nGot: %v", tc.wantNS[i], got)
					}
				}
			}
		})
	}
}
