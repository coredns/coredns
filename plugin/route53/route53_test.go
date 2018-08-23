package route53

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/plugin/test"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	"github.com/miekg/dns"
)

type fakeRoute53 struct {
	route53iface.Route53API
}

func (fakeRoute53) ListHostedZonesByNameWithContext(_ aws.Context, input *route53.ListHostedZonesByNameInput, _ ...request.Option) (*route53.ListHostedZonesByNameOutput, error) {
	return nil, nil
}

func (fakeRoute53) ListResourceRecordSetsPagesWithContext(_ aws.Context, input *route53.ListResourceRecordSetsInput, fn func(*route53.ListResourceRecordSetsOutput, bool) bool, _ ...request.Option) error {
	var rrs []*route53.ResourceRecordSet
	for _, r := range []struct {
		rType, name, value string
	}{
		{"A", "example.org.", "1.2.3.4"},
		{"AAAA", "example.org.", "2001:db8:85a3::8a2e:370:7334"},
		{"CNAME", "sample.example.org.", "example.org"},
		{"PTR", "example.org.", "ptr.example.org"},
		{"SOA", "org.", "ns-1536.awsdns-00.co.uk. awsdns-hostmaster.amazon.com. 1 7200 900 1209600 86400"},
	} {
		rrs = append(rrs, &route53.ResourceRecordSet{Type: aws.String(r.rType),
			Name: aws.String(r.name),
			ResourceRecords: []*route53.ResourceRecord{
				{
					Value: aws.String(r.value),
				},
			},
			TTL: aws.Int64(300),
		})
	}
	if ok := fn(&route53.ListResourceRecordSetsOutput{
		ResourceRecordSets: rrs,
	}, true); !ok {
		return errors.New("pagin function return false")
	}
	return nil
}

func TestRoute53(t *testing.T) {
	ctx := context.Background()
	r, err := NewRoute53(ctx, fakeRoute53{}, map[string]string{"org.": "1234567890"}, &upstream.Upstream{})
	if err != nil {
		t.Fatalf("Failed to create Route53: %v", err)
	}
	r.Next = test.ErrorHandler()
	err = r.Run(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize Route53: %v", err)
	}

	tests := []struct {
		qname        string
		qtype        uint16
		expectedCode int
		wantAnswer   []string // ownernames for the records in the additional section.
		wantNs       []string
		expectedErr  error
	}{
		// 0. example.org A found - success.
		{
			qname:        "example.org",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			wantAnswer: []string{"example.org.	300	IN	A	1.2.3.4"},
		},
		// 1. example.org AAAA found - success.
		{
			qname:        "example.org",
			qtype:        dns.TypeAAAA,
			expectedCode: dns.RcodeSuccess,
			wantAnswer: []string{"example.org.	300	IN	AAAA	2001:db8:85a3::8a2e:370:7334"},
		},
		// 2. exampled.org PTR found - success.
		{
			qname:        "example.org",
			qtype:        dns.TypePTR,
			expectedCode: dns.RcodeSuccess,
			wantAnswer: []string{"example.org.	300	IN	PTR	ptr.example.org"},
		},
		// 3. sample.example.org points to example.org CNAME.
		// Query must return both CNAME and A recs.
		{
			qname:        "sample.example.org",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			wantAnswer: []string{
				"sample.example.org.	300	IN	CNAME	example.org.",
				"example.org.	300	IN	A	1.2.3.4",
			},
		},
		// 4. Explicit CNAME query for sample.example.org.
		// Query must return just CNAME.
		{
			qname:        "sample.example.org",
			qtype:        dns.TypeCNAME,
			expectedCode: dns.RcodeSuccess,
			wantAnswer: []string{"sample.example.org.	300	IN	CNAME	example.org."},
		},
		// 5. Zone not configured.
		{
			qname:        "badexample.com",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeServerFailure,
		},
		// 6. No record found. Return SOA record.
		{
			qname:        "bad.org",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			wantNs: []string{"org.	300	IN	SOA	ns-1536.awsdns-00.co.uk. awsdns-hostmaster.amazon.com. 1 7200 900 1209600 86400"},
		},
	}

	for ti, tc := range tests {
		req := new(dns.Msg)
		req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		code, err := r.ServeDNS(ctx, rec, req)

		if err != tc.expectedErr {
			t.Errorf("Test %d: Expected error %v, but got %v", ti, tc.expectedErr, err)
		}
		if code != int(tc.expectedCode) {
			t.Errorf("Test %d: Expected status code %d, but got %d", ti, tc.expectedCode, code)
		}

		if len(tc.wantAnswer) != len(rec.Msg.Answer) {
			t.Errorf("Test %d: Unexpected number of Answers. Want: %d, got: %d", ti, len(tc.wantAnswer), len(rec.Msg.Answer))
		} else {
			for i, gotAnswer := range rec.Msg.Answer {
				if gotAnswer.String() != tc.wantAnswer[i] {
					t.Errorf("Test %d: Expected answer. Want: %s, got: %s", ti, tc.wantAnswer[i], gotAnswer)
				}
			}
		}

		if len(tc.wantNs) != len(rec.Msg.Ns) {
			t.Errorf("Test %d: Unexpected NS number. Want: %d, got: %d", ti, len(tc.wantNs), len(rec.Msg.Ns))
		} else {
			for i, ns := range rec.Msg.Ns {
				got, ok := ns.(*dns.SOA)
				if !ok {
					t.Errorf("Test %d: Unexpected NS type. Want: SOA, got: %v", ti, reflect.TypeOf(got))
				}
				if got.String() != tc.wantNs[i] {
					t.Errorf("Test %d: Unexpected NS. Want: %v, got: %v", ti, tc.wantNs[i], got)
				}
			}
		}
	}
}
