package rewrite

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

// ResponseRule contains a rule to rewrite a response with.
type ResponseRule struct {
	Active      bool
	Type        string
	Pattern     *regexp.Regexp
	Replacement string
	TTL         uint32
}

// ResponseReverter reverses the operations done on the question section of a packet.
// This is need because the client will otherwise disregards the response, i.e.
// dig will complain with ';; Question section mismatch: got example.org/HINFO/IN'
type ResponseReverter struct {
	dns.ResponseWriter
	originalQuestion dns.Question
	ResponseRewrite  bool
	ResponseRules    []ResponseRule
}

// NewResponseReverter returns a pointer to a new ResponseReverter.
func NewResponseReverter(w dns.ResponseWriter, r *dns.Msg) *ResponseReverter {
	return &ResponseReverter{
		ResponseWriter:   w,
		originalQuestion: r.Question[0],
	}
}

// WriteMsg records the status code and calls the underlying ResponseWriter's WriteMsg method.
func (r *ResponseReverter) WriteMsg(res *dns.Msg) error {
	res.Question[0] = r.originalQuestion
	if r.ResponseRewrite {
		for _, rr := range res.Ns {
			rewriteResourceRecord(res, rr, r)
		}

		for _, rr := range res.Answer {
			rewriteResourceRecord(res, rr, r)
		}

		for _, rr := range res.Extra {
			rewriteResourceRecord(res, rr, r)
		}

	}
	return r.ResponseWriter.WriteMsg(res)
}

func rewriteResourceRecord(res *dns.Msg, rr dns.RR, r *ResponseReverter) {
	var (
		isNameRewritten   bool
		isTTLRewritten    bool
		isTargetRewritten bool
		name              = rr.Header().Name
		ttl               = rr.Header().Ttl
		target            string
	)

	target = getTargetNameForRewrite(rr)

	for _, rule := range r.ResponseRules {
		if rule.Type == "" {
			rule.Type = "name"
		}
		switch rule.Type {
		case "name":
			rewriteName(rule, &name, &isNameRewritten)
			if target != "" {
				rewriteName(rule, &target, &isTargetRewritten)
			}
		case "ttl":
			ttl = rule.TTL
			isTTLRewritten = true
		}
	}
	if isNameRewritten {
		rr.Header().Name = name
	}
	if isTTLRewritten {
		rr.Header().Ttl = ttl
	}
	if isTargetRewritten {
		setRewrittenTargetName(rr, target)
	}
}

func getTargetNameForRewrite(rr dns.RR) (name string) {
	switch rr.Header().Rrtype {
	case dns.TypeSRV:
		return rr.(*dns.SRV).Target
	case dns.TypeMX:
		return rr.(*dns.MX).Mx
	case dns.TypeCNAME:
		return rr.(*dns.CNAME).Target
	case dns.TypeNS:
		return rr.(*dns.NS).Ns
	case dns.TypeDNAME:
		return rr.(*dns.DNAME).Target
	case dns.TypeNAPTR:
		return rr.(*dns.NAPTR).Replacement
	case dns.TypeSOA:
		return rr.(*dns.SOA).Ns
	}

	return ""
}

func setRewrittenTargetName(rr dns.RR, name string) {
	switch rr.Header().Rrtype {
	case dns.TypeSRV:
		rr.(*dns.SRV).Target = name
	case dns.TypeMX:
		rr.(*dns.MX).Mx = name
	case dns.TypeCNAME:
		rr.(*dns.CNAME).Target = name
	case dns.TypeNS:
		rr.(*dns.NS).Ns = name
	case dns.TypeDNAME:
		rr.(*dns.DNAME).Target = name
	case dns.TypeNAPTR:
		rr.(*dns.NAPTR).Replacement = name
	case dns.TypeSOA:
		rr.(*dns.SOA).Ns = name
	}

}

func rewriteName(rule ResponseRule, name *string, isNameRewritten *bool) {
	regexGroups := rule.Pattern.FindStringSubmatch(*name)
	if len(regexGroups) == 0 {
		return
	}
	s := rule.Replacement
	for groupIndex, groupValue := range regexGroups {
		groupIndexStr := "{" + strconv.Itoa(groupIndex) + "}"
		s = strings.Replace(s, groupIndexStr, groupValue, -1)
	}

	*isNameRewritten = true
	*name = s
}

// Write is a wrapper that records the size of the message that gets written.
func (r *ResponseReverter) Write(buf []byte) (int, error) {
	n, err := r.ResponseWriter.Write(buf)
	return n, err
}
