package route53

import (
	"context"
	"github.com/miekg/dns"
)

// AliasResolver helps us in resolving ALIAS records from AWS.
type AliasResolver interface {
	Resolve(ctx context.Context, dnsName string, dnsType uint16, ns string, ttl int64) (rrs []dns.RR, err error)
}

type authoritativeNsResolver struct{}

func (anr authoritativeNsResolver) Resolve(ctx context.Context, dnsName string, dnsType uint16, ns string, ttl int64) (rrs []dns.RR, err error) {
	client := new(dns.Client)
	req := new(dns.Msg)

	req.RecursionDesired = true
	req.SetQuestion(dnsName, dnsType)

	r, _, err := client.ExchangeContext(ctx, req, ns)
	if err != nil {
		return
	}

	for _, resp := range r.Answer {
		var rec dns.RR
		switch response := resp.(type) {
		case *dns.A:
			rec, _ = remapDnsAliasRR(dnsName, response, ttl)
		case *dns.AAAA:
			rec, _ = remapDnsAliasRR(dnsName, response, ttl)
		}
		rrs = append(rrs, rec)
	}

	return
}
