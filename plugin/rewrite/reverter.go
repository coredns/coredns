package rewrite

import (
	"github.com/miekg/dns"
)

// RevertPolicy controls the overall reverting process
type RevertPolicy interface {
	DoRevert() bool
	DoQuestionRestore() bool
}

type revertPolicy struct {
	noRevert  bool
	noRestore bool
}

func (p revertPolicy) DoRevert() bool {
	return !p.noRevert
}

func (p revertPolicy) DoQuestionRestore() bool {
	return !p.noRestore
}

// NoRevertPolicy disables all response rewrite rules
func NoRevertPolicy() RevertPolicy {
	return revertPolicy{true, false}
}

// NoRestorePolicy disables the question restoration during the response rewrite
func NoRestorePolicy() RevertPolicy {
	return revertPolicy{false, true}
}

// NewRevertPolicy creates a new reverter policy by dynamically specifying all
// options.
func NewRevertPolicy(noRevert, noRestore bool) RevertPolicy {
	return revertPolicy{noRestore: noRestore, noRevert: noRevert}
}

// ResponseRule contains a rule to rewrite a response with.
type ResponseRule interface {
	RewriteResponse(res *dns.Msg, rr dns.RR)
}

// ResponseRules describes an ordered list of response rules to apply
// after a name rewrite
type ResponseRules = []ResponseRule

// ResponseReverter reverses the operations done on the question section of a packet.
// This is need because the client will otherwise disregards the response, i.e.
// dig will complain with ';; Question section mismatch: got example.org/HINFO/IN'
type ResponseReverter struct {
	dns.ResponseWriter
	originalQuestion dns.Question
	ResponseRules    ResponseRules
	revertPolicy     RevertPolicy
}

// NewResponseReverter returns a pointer to a new ResponseReverter.
func NewResponseReverter(w dns.ResponseWriter, r *dns.Msg, policy RevertPolicy) *ResponseReverter {
	return &ResponseReverter{
		ResponseWriter:   w,
		originalQuestion: r.Question[0],
		revertPolicy:     policy,
	}
}

// WriteMsg records the status code and calls the underlying ResponseWriter's WriteMsg method.
func (r *ResponseReverter) WriteMsg(res1 *dns.Msg) error {
	// Deep copy 'res' as to not (e.g). rewrite a message that's also stored in the cache.
	res := res1.Copy()

	if r.revertPolicy.DoQuestionRestore() {
		res.Question[0] = r.originalQuestion
	}
	if len(r.ResponseRules) > 0 {
		for _, rr := range res.Ns {
			r.rewriteResourceRecord(res, rr)
		}
		for _, rr := range res.Answer {
			r.rewriteResourceRecord(res, rr)
		}
		for _, rr := range res.Extra {
			r.rewriteResourceRecord(res, rr)
		}
	}
	return r.ResponseWriter.WriteMsg(res)
}

func (r *ResponseReverter) rewriteResourceRecord(res *dns.Msg, rr dns.RR) {
	// The reverting rules need to be done in reversed order.
	for i := len(r.ResponseRules) - 1; i >= 0; i-- {
		r.ResponseRules[i].RewriteResponse(res, rr)
	}
}

// Write is a wrapper that records the size of the message that gets written.
func (r *ResponseReverter) Write(buf []byte) (int, error) {
	n, err := r.ResponseWriter.Write(buf)
	return n, err
}

func getRecordValueForRewrite(rr dns.RR) (name string) {
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
	case dns.TypePTR:
		return rr.(*dns.PTR).Ptr
	default:
		return ""
	}
}

func setRewrittenRecordValue(rr dns.RR, value string) {
	switch rr.Header().Rrtype {
	case dns.TypeSRV:
		rr.(*dns.SRV).Target = value
	case dns.TypeMX:
		rr.(*dns.MX).Mx = value
	case dns.TypeCNAME:
		rr.(*dns.CNAME).Target = value
	case dns.TypeNS:
		rr.(*dns.NS).Ns = value
	case dns.TypeDNAME:
		rr.(*dns.DNAME).Target = value
	case dns.TypeNAPTR:
		rr.(*dns.NAPTR).Replacement = value
	case dns.TypeSOA:
		rr.(*dns.SOA).Ns = value
	case dns.TypePTR:
		rr.(*dns.PTR).Ptr = value
	}
}
