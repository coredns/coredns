// Package route53 implements a plugin that returns resource records
// from AWS route53
package route53

import (
	"context"
	"fmt"
	"net"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/request"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	"github.com/miekg/dns"
)

// Route53 is a plugin that returns RR from AWS route53
type Route53 struct {
	Next plugin.Handler

	upstream upstream.Upstream
	zones    []string
	keys     map[string]string
	client   route53iface.Route53API
	writer   dns.ResponseWriter
	ctx      context.Context
}

// ServeDNS implements the plugin.Handler interface.
func (r53 Route53) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	r53.upstream, _ = upstream.NewUpstream([]string{})
	r53.writer = w
	r53.ctx = ctx

	state := request.Request{W: w, Req: r, Context: ctx}
	qname := state.Name()

	zone := plugin.Zones(r53.zones).Matches(qname)
	if zone == "" {
		return plugin.NextOrFailure(r53.Name(), r53.Next, ctx, w, r)
	}

	output, err := r53.client.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(r53.keys[zone]),
		StartRecordName: aws.String(qname),
		StartRecordType: aws.String(state.Type()),
		MaxItems:        aws.String("1"),
	})
	if err != nil {
		return dns.RcodeServerFailure, err
	}

	fmt.Printf("Output from aws: %+v\n\n", output)

	answers := []dns.RR{}
	switch state.QType() {
	case dns.TypeA:
		answers = r53.a(qname, output.ResourceRecordSets)
	case dns.TypeAAAA:
		answers = aaaa(qname, output.ResourceRecordSets)
	case dns.TypePTR:
		answers = ptr(qname, output.ResourceRecordSets)
	case dns.TypeCNAME:
		answers = r53.cname(state, qname, output.ResourceRecordSets)
	default:
		fmt.Printf("We defaulted. %+v\n", state)
	}

	if len(answers) == 0 {
		return plugin.NextOrFailure(r53.Name(), r53.Next, ctx, w, r)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, true, true
	m.Answer = answers

	state.SizeAndDo(m)
	m, _ = state.Scrub(m)
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

func (r53 *Route53) a(zone string, rrss []*route53.ResourceRecordSet) []dns.RR {
	answers := []dns.RR{}
	fmt.Printf("Recordset: %+v\n\n", rrss)
	for _, rrs := range rrss {
		if *rrs.Type == "A" {
			for _, rr := range rrs.ResourceRecords {
				r := new(dns.A)
				r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(aws.Int64Value(rrs.TTL))}
				r.A = net.ParseIP(aws.StringValue(rr.Value)).To4()
				answers = append(answers, r)
			}
		} else {
			for _, rr := range rrs.ResourceRecords {
				req := new(dns.Msg)
				req.SetQuestion(*rr.Value, dns.StringToType[*rrs.Type])
				upstreamRequest := request.Request{W: r53.writer, Req: req, Context: r53.ctx}
				upstreamResponse, _ := r53.upstream.Lookup(upstreamRequest, *rr.Value, dns.StringToType[*rrs.Type])
				fmt.Printf("We get this dns msg back: %+v\n\n", upstreamResponse)
			}
		}
	}
	return answers
}

func aaaa(zone string, rrss []*route53.ResourceRecordSet) []dns.RR {
	answers := []dns.RR{}
	for _, rrs := range rrss {
		for _, rr := range rrs.ResourceRecords {
			r := new(dns.AAAA)
			r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: uint32(aws.Int64Value(rrs.TTL))}
			r.AAAA = net.ParseIP(aws.StringValue(rr.Value)).To16()
			answers = append(answers, r)
		}
	}
	return answers
}

func ptr(zone string, rrss []*route53.ResourceRecordSet) []dns.RR {
	answers := []dns.RR{}
	for _, rrs := range rrss {
		for _, rr := range rrs.ResourceRecords {
			r := new(dns.PTR)
			r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: uint32(aws.Int64Value(rrs.TTL))}
			r.Ptr = aws.StringValue(rr.Value)
			answers = append(answers, r)
		}
	}
	return answers
}

func (r53 *Route53) cname(state request.Request, zone string, rrss []*route53.ResourceRecordSet) []dns.RR {
	defer func() {
		if argh := recover(); argh != nil {
			fmt.Println("Recovered in f", argh)
		}
	}()
	answers := []dns.RR{}
	for _, rrs := range rrss {
		for _, rr := range rrs.ResourceRecords {
			fmt.Printf("state: %+v\n, zone %+v\n", state, zone)
			m, e := r53.upstream.Lookup(state, aws.StringValue(rr.Value)+".", dns.TypeA)
			fmt.Printf("m: %+v\ne: %+v\n", m, e)

			if e != nil {
				continue
			}
			answers = append(answers, m.Answer...)
		}
	}
	return answers
}

/*func srv(zone string, rrss []*route53.ResourceRecordSet) []dns.RR {
	answers := []dns.RR{}
	return answer
}*/

// Name implements the Handler interface.
func (r53 Route53) Name() string { return "route53" }
