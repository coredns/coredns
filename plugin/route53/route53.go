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
	fmt.Printf("\n\nserve it up\n\n")
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

	//fmt.Printf("State qtype: %+v\n", state.QType())

	answers := []dns.RR{}
	switch state.QType() {
	case dns.TypeA:
		answers = r53.answer(state, qname, output.ResourceRecordSets)
	case dns.TypeAAAA:
		answers = aaaa(qname, output.ResourceRecordSets)
	case dns.TypePTR:
		answers = ptr(qname, output.ResourceRecordSets)
	case dns.TypeCNAME:
		answers = r53.cname(state, qname, output.ResourceRecordSets)
		//answers = r53.cname(state, qname, output.ResourceRecordSets)
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
	fmt.Printf("Writing message: %+v\n", m)
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

func (r53 *Route53) answer(state request.Request, zone string, rrss []*route53.ResourceRecordSet) []dns.RR {
	answers := []dns.RR{}
	for _, rrs := range rrss {
		if *rrs.Type == dns.TypeToString[uint16(state.QType())] {
			fmt.Printf("Resource recordset length: %+v\n", len(rrs.ResourceRecords))
			for _, rr := range rrs.ResourceRecords {
				r := createAnswer(zone, []string{}, *rrs.Type, rr, uint32(aws.Int64Value(rrs.TTL)))
				answers = append(answers, r)
			}
		} else {
			for _, rr := range rrs.ResourceRecords {
				fmt.Printf("upstreaming request %+v\n", *rr.Value)
				upstreamResponse, e := r53.upstream.Lookup(state, aws.StringValue(rr.Value)+".", dns.StringToType[*rrs.Type])
				if e != nil {
					fmt.Printf("upstream lookup failed...\n")
					continue
				}
				answers = append(answers, upstreamResponse.Answer...)
			}
		}
	}
	return answers
}

/*func (r53 *Route53) a(state request.Request, zone string, rrss []*route53.ResourceRecordSet) []dns.RR {
	answers := []dns.RR{}
	//fmt.Printf("Recordset: %+v\n\n", rrss)
	for _, rrs := range rrss {
		if *rrs.Type == "A" {
			for _, rr := range rrs.ResourceRecords {
				r := createAnswer(zone, []string{}, *rrs.Type, rr, uint32(aws.Int64Value(rrs.TTL)))
				answers = append(answers, r)
			}
		} else {
			for _, rr := range rrs.ResourceRecords {
				fmt.Printf("upstreaming request %+v\n", *rr.Value)
				upstreamResponse, e := r53.upstream.Lookup(state, aws.StringValue(rr.Value)+".", dns.StringToType[*rrs.Type])
				if e != nil {
					fmt.Printf("upstream lookup failed...\n")
					continue
				}
				answers = append(answers, upstreamResponse.Answer...)
			}
		}
	}
	return answers
}*/

func createAnswer(zone string, reqHistory []string, requestedType string, resourceRecord *route53.ResourceRecord, ttl uint32) dns.RR {
	answerType := dns.StringToType[requestedType]
	answerRecord := dns.TypeToRR[uint16(answerType)]()

	switch r := answerRecord.(type) {
	case *dns.A:
		fmt.Printf("Type is A\n")
		r.Hdr = dns.RR_Header{Name: zone, Rrtype: uint16(answerType), Class: dns.ClassINET, Ttl: ttl}
		r.A = net.ParseIP(aws.StringValue(resourceRecord.Value)).To4()
	case *dns.AAAA:
		fmt.Printf("Type is AAAA\n")
		r.Hdr = dns.RR_Header{Name: zone, Rrtype: uint16(answerType), Class: dns.ClassINET, Ttl: ttl}
		r.AAAA = net.ParseIP(aws.StringValue(resourceRecord.Value)).To16()
	case *dns.CNAME:
		fmt.Printf("Type is CNAME\n")
		r.Hdr = dns.RR_Header{Name: zone, Rrtype: uint16(answerType), Class: dns.ClassINET, Ttl: ttl}
		r.Target = aws.StringValue(resourceRecord.Value)
	default:
		fmt.Printf("Should have upstreamed this one")
	}
	//fmt.Printf("Leaving createAnswer\n")
	return answerRecord
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
			//fmt.Printf("state: %+v\n, zone %+v\n", state, zone)
			m, e := r53.upstream.Lookup(state, aws.StringValue(rr.Value)+".", dns.StringToType[*rrs.Type])
			//fmt.Printf("m: %+v\ne: %+v\n", m, e)

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
