package recursion

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("recursion")

const defaultTimeout = 10 * time.Second

// ResponseRecursionWriter is a response writer that allows modifying dns.MsgHdr
type ResponseRecursionWriter struct {
	dns.ResponseWriter
	maxDepth uint32
	tries    uint32
	qType    uint16
	qClass   uint16
	ctx      context.Context

	next plugin.Handler
}

// WriteMsg implements the dns.ResponseWriter interface.
func (r *ResponseRecursionWriter) WriteMsg(res *dns.Msg) error {
	// The response has already been recursively handled
	if res.RecursionAvailable {
		return r.ResponseWriter.WriteMsg(res)
	}

	res.RecursionAvailable = true // Avoid loops

	// Dedup all records for conciseness
	dedupMap := make(map[string]dns.RR)
	res.Answer = dns.Dedup(res.Answer, dedupMap)

	CNAMEs := getCnames(res.Answer)

	// If there are no CNAME entries to lookup or the type has already been provided
	if len(CNAMEs) == 0 || hasType(res.Answer, r.qType) {
		return r.ResponseWriter.WriteMsg(res)
	}

	// Loop a number of tries until a record type is found
	hasAlternates := len(CNAMEs) > 1
	var rcode int
	var err error
	var answers []dns.RR
	cachedQuery := make(map[string][]dns.RR)

recursionRetry:
	for ; r.tries > 0; r.tries-- {
		answers = append([]dns.RR{}, res.Answer...)
		next := &dns.Msg{Question: []dns.Question{{Name: CNAMEs[rand.Intn(len(CNAMEs))].Target, Qclass: r.qClass, Qtype: r.qType}}}

		for depth := r.maxDepth; depth > 0; depth-- {
			if err = r.ctx.Err(); err != nil {
				rcode = dns.RcodeServerFailure
				break recursionRetry
			}

			var subAnswer []dns.RR
			var ok bool

			// Prevent querying the same lookup twice in the same recursive call
			if subAnswer, ok = cachedQuery[next.Question[0].Name]; !ok {
				subQry := nonwriter.New(r.ResponseWriter)
				subQueryCount.Add(1)
				rcode, err = plugin.NextOrFailure(name, r.next, r.ctx, subQry, next)
				if rcode != dns.RcodeSuccess {
					continue recursionRetry
				}

				subAnswer = subQry.Msg.Answer
				cachedQuery[next.Question[0].Name] = subAnswer
			}

			// Compile the answers all together
			answers = append(answers, subAnswer...)
			answers := dns.Dedup(answers, dedupMap)
			subCNAMEs := getCnames(subAnswer)

			// If alternate CNAMES options exist, enable retries
			if len(subCNAMEs) > 1 {
				hasAlternates = true
			}

			if hasType(subAnswer, r.qType) || (len(subCNAMEs) == 0 && !hasAlternates) {
				res.RecursionAvailable = true
				res.Answer = answers
				return r.ResponseWriter.WriteMsg(res)
			}
			next.Question[0].Name = subCNAMEs[rand.Intn(len(subCNAMEs))].Target
		}
	}

	if rcode != dns.RcodeSuccess {
		res.Answer = answers
		res.Rcode = rcode
	} else {
		res.Rcode = dns.RcodeServerFailure
	}

	r.ResponseWriter.WriteMsg(res)
	if err != nil {
		return fmt.Errorf("recursion failed, %s", err)
	}
	return fmt.Errorf("recursion failed, tries exhaused")
}

func getCnames(rr []dns.RR) (ret []*dns.CNAME) {
	for _, r := range rr {
		if cn, ok := r.(*dns.CNAME); ok {
			ret = append(ret, cn)
		}
	}
	return
}

func hasType(rr []dns.RR, qType uint16) bool {
	for _, r := range rr {
		if r.Header().Rrtype == qType {
			return true
		}
	}
	return false
}

// Write implements the dns.ResponseWriter interface.
func (r *ResponseRecursionWriter) Write(buf []byte) (int, error) {
	log.Warning("ResponseRecursionWriter called with Write: not ensuring recursion is handled")
	return r.ResponseWriter.Write(buf)
}

func (f *Recursion) match(state request.Request) bool {
	if !f.isAllowedDomain(state.Name()) {
		return false
	}

	return true
}

func (f *Recursion) isAllowedDomain(name string) bool {
	for _, ignore := range f.ignored {
		if plugin.Name(ignore).Matches(name) {
			return false
		}
	}
	return true
}
