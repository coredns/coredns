package route53

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/test"
	crequest "github.com/coredns/coredns/request"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	"github.com/miekg/dns"
)

type fakeAliasResolver struct{}

func (far fakeAliasResolver) Resolve(ctx context.Context, dnsName string, dnsType uint16, ns string, ttl int64) (rrs []dns.RR, err error) {
	val := "1.2.3.4"
	rr, _ := rrFromRR(dnsName, "A", ttl, route53.ResourceRecord{Value: &val})
	rrs = append(rrs, rr)
	return
}

type fakeRoute53Alias struct {
	route53iface.Route53API
}

func (fakeRoute53Alias) ListHostedZonesByNameWithContext(_ aws.Context, input *route53.ListHostedZonesByNameInput, _ ...request.Option) (*route53.ListHostedZonesByNameOutput, error) {
	return nil, nil
}

func (fakeRoute53Alias) ListResourceRecordSetsPagesWithContext(_ aws.Context, in *route53.ListResourceRecordSetsInput, fn func(*route53.ListResourceRecordSetsOutput, bool) bool, _ ...request.Option) error {
	if aws.StringValue(in.HostedZoneId) == "0987654321" {
		return errors.New("bad. zone is bad")
	}
	rrsResponse := map[string][]*route53.ResourceRecordSet{}
	for _, r := range []struct {
		rType, name, value, hostedZoneID string
		aliasDnsName, aliasHostedZoneId  string
		alias                            bool
	}{
		{"A", "alias.example.dev.", "1.2.3.4", "1234567891", "random.alias.address.", "3234567891", true},
		{"A", "same-zone-alias.example.dev.", "4.3.2.1", "1234567891", "target.example.dev.", "1234567891", true},
		{"A", "example.dev.", "1.2.3.4", "1234567891", "", "", false},
		{"A", "target.example.dev.", "4.3.2.1", "1234567891", "", "", false},
		{"A", "target.example.dev.", "8.7.6.5", "1234567891", "", "", false},
		{"SOA", "dev.", "ns-1536.awsdns-00.co.uk. awsdns-hostmaster.amazon.com. 1 7200 900 1209600 86400", "1234567891", "", "", false},
		{"NS", "dev.", "ns-1536.awsdns-00.co.uk", "1234567891", "", "", false},
		// ALIAS record
	} {
		rrs, ok := rrsResponse[r.hostedZoneID]
		if !ok {
			rrs = make([]*route53.ResourceRecordSet, 0)
		}
		record := route53.ResourceRecordSet{Type: aws.String(r.rType),
			Name: aws.String(r.name),
			ResourceRecords: []*route53.ResourceRecord{
				{
					Value: aws.String(r.value),
				},
			},
			TTL: aws.Int64(300),
		}
		if r.alias {
			record.ResourceRecords = nil // we need to reset it to empty, an alias record would not have data in it
			dnsName := r.aliasDnsName
			hostedZone := r.aliasHostedZoneId
			evaluateTargetHealth := false

			record.AliasTarget = &route53.AliasTarget{
				DNSName:              &dnsName,
				EvaluateTargetHealth: &evaluateTargetHealth,
				HostedZoneId:         &hostedZone,
			}
		}

		rrs = append(rrs, &record)
		rrsResponse[r.hostedZoneID] = rrs
	}

	if ok := fn(&route53.ListResourceRecordSetsOutput{
		ResourceRecordSets: rrsResponse[aws.StringValue(in.HostedZoneId)],
	}, true); !ok {
		return errors.New("paging function return false")
	}
	return nil
}

func TestRoute53WithAlias(t *testing.T) {
	ctx := context.Background()

	r, err := New(ctx, fakeRoute53Alias{}, map[string][]string{"dev.": {"1234567891"}}, 90*time.Second, fakeAliasResolver{})
	if err != nil {
		t.Fatalf("Failed to create Route53: %v", err)
	}
	r.Fall = fall.Zero
	r.Fall.SetZonesFromArgs([]string{"gov."})
	r.Next = test.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		state := crequest.Request{W: w, Req: r}
		qname := state.Name()
		m := new(dns.Msg)
		rcode := dns.RcodeServerFailure
		if qname == "example.gov." {
			m.SetReply(r)
			rr, err := dns.NewRR("example.gov.  300 IN  A   2.4.6.8")
			if err != nil {
				t.Fatalf("Failed to create Resource Record: %v", err)
			}
			m.Answer = []dns.RR{rr}

			m.Authoritative = true
			rcode = dns.RcodeSuccess

		}

		m.SetRcode(r, rcode)
		w.WriteMsg(m)
		return rcode, nil
	})
	err = r.Run(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize Route53: %v", err)
	}

	tests := []struct {
		qname        string
		qtype        uint16
		wantRetCode  int
		wantAnswer   []string // ownernames for the records in the additional section.
		wantMsgRCode int
		wantNS       []string
		expectedErr  error
	}{
		{
			qname: "example.dev",
			qtype: dns.TypeA,
			wantAnswer: []string{
				"example.dev.	300	IN	A	1.2.3.4",
			},
		},
		// 15. alias.example.dev points to 1.2.3.4.
		{
			qname: "alias.example.dev",
			qtype: dns.TypeA,
			wantAnswer: []string{
				"alias.example.dev.	60	IN	A	1.2.3.4",
			},
		},
		// 16. same-zone-alias.example.dev points to 4.3.2.1.
		{
			qname: "same-zone-alias.example.dev",
			qtype: dns.TypeA,
			wantAnswer: []string{
				"same-zone-alias.example.dev.	300	IN	A	4.3.2.1",
				"same-zone-alias.example.dev.	300	IN	A	8.7.6.5",
			},
		},
	}

	for ti, tc := range tests {
		req := new(dns.Msg)
		req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		code, err := r.ServeDNS(ctx, rec, req)

		if err != tc.expectedErr {
			t.Fatalf("Test %d: Expected error %v, but got %v", ti, tc.expectedErr, err)
		}
		if code != int(tc.wantRetCode) {
			t.Fatalf("Test %d: Expected returned status code %s, but got %s", ti, dns.RcodeToString[tc.wantRetCode], dns.RcodeToString[code])
		}

		if tc.wantMsgRCode != rec.Msg.Rcode {
			t.Errorf("Test %d: Unexpected msg status code. Want: %s, got: %s", ti, dns.RcodeToString[tc.wantMsgRCode], dns.RcodeToString[rec.Msg.Rcode])
		}

		if len(tc.wantAnswer) != len(rec.Msg.Answer) {
			t.Errorf("Test %d: Unexpected number of Answers. Want: %d, got: %d", ti, len(tc.wantAnswer), len(rec.Msg.Answer))
		} else {
			for i, gotAnswer := range rec.Msg.Answer {
				if gotAnswer.String() != tc.wantAnswer[i] {
					t.Errorf("Test %d: Unexpected answer.\nWant:\n\t%s\nGot:\n\t%s", ti, tc.wantAnswer[i], gotAnswer)
				}
			}
		}
	}
}
