package autopath

import (
	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/pkg/dnsrecorder"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

func (ap AutoPath) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	// Nothing to intercept.
	if ap.Next == nil {
		return middleware.NextOrFailure(ap.Name(), ap.Next, ctx, w, r)
	}
	// Only handle A and AAAA queries.
	if r.Question[0].Qtype != dns.TypeA && r.Question[0].Qtype != dns.TypeAAAA {
		return middleware.NextOrFailure(ap.Name(), ap.Next, ctx, w, r)
	}

	qn := r.Question[0].Name

	// Look if the name falls beneath a search path, if so assume it was appended
	// and we will traverse the search path and the client's behalf.
	base := ""
	for _, search := range ap.SearchPath {
		trunc, ok := withinSearchPath(qn, search)
		if !ok {
			continue
		}
		if dns.CountLabel(trunc) <= ap.Ndots {
			continue
		}
		base = trunc
		break
	}

	// Build a list of names we need to query until we get an answer.
	names := []string{}
	for _, search := range ap.SearchPath {
		names = append(names, base+"."+search)
	}
	// base name as well
	names = append(names, dns.Fqdn(base))

	recw := dnsrecorder.New(w)
	for _, n := range names {
		r.Question[0].Name = n
		ret, err := ap.Next.ServeDNS(ctx, recw, r)
		if err != nil {
			continue
		}
		if middleware.ClientWriteDone(ret) {
			if recw.Msg.Rcode == dns.RcodeSuccess {
				cnameChainAnswer(qn, recw.Msg)
				w.WriteMsg(recw.Msg)
				return dns.RcodeSuccess, nil
			}
		}
	}

	return dns.RcodeSuccess, nil
}

// Name implements the Handler interface.
func (ap AutoPath) Name() string { return "autopath" }

// withinSearchPath return true when qname sits below the search path. truncated
// hold the name with search subtracted.
// TODO(miek): tests.
func withinSearchPath(qname, search string) (truncated string, ok bool) {
	if dns.IsSubDomain(search, qname) {
		return qname[:len(qname)-len(search)-1], true
	}
	return qname, false

}

// cnameChainAnswer create CNAME in the answer to "glue back" the correct response to the client
func cnameChainAnswer(orig string, r *dns.Msg) {
	for _, a := range r.Answer {
		if orig == a.Header().Name {
			continue
		}
		// prepend the cname
		cname := &dns.CNAME{Hdr: dns.RR_Header{Name: orig, Rrtype: dns.TypeCNAME, Class: a.Header().Class, Ttl: a.Header().Ttl}, Target: a.Header().Name}
		r.Answer = append([]dns.RR{cname}, r.Answer...)
	}
}
