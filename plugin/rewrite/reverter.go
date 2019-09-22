package rewrite

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

// ResponseRuleType is the type of a ResponseRule
type ResponseRuleType string

const (
	// ResponseRuleTypeUnset is the unset response rule type; it defaults to
	// setting the answer to match the question.
	// It uses no fields of the ResponseRule struct.
	ResponseRuleTypeUnset ResponseRuleType = ""
	// ResponseRuleTypeName is a name type. It uses the Pattern and Replacement
	// fields of ResponseRule
	ResponseRuleTypeName = "name"
	// ResponseRuleTypeTTL is the type of ResponseRule that rewrites the TTL. It uses the TTL field.
	ResponseRuleTypeTTL = "ttl"
)

// ResponseRule contains a rule to rewrite a response with.
type ResponseRule struct {
	Type        ResponseRuleType
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
		for _, rr := range res.Answer {
			var (
				isNameRewritten bool
				isTTLRewritten  bool
				name            = rr.Header().Name
				ttl             = rr.Header().Ttl
			)
			for _, rule := range r.ResponseRules {
				switch rule.Type {
				case ResponseRuleTypeUnset:
					name = r.originalQuestion.Name
					isNameRewritten = true
				case ResponseRuleTypeName:
					regexGroups := rule.Pattern.FindStringSubmatch(name)
					if len(regexGroups) == 0 {
						continue
					}
					s := rule.Replacement
					for groupIndex, groupValue := range regexGroups {
						groupIndexStr := "{" + strconv.Itoa(groupIndex) + "}"
						s = strings.Replace(s, groupIndexStr, groupValue, -1)
					}
					name = s
					isNameRewritten = true
				case ResponseRuleTypeTTL:
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
		}
	}
	return r.ResponseWriter.WriteMsg(res)
}

// Write is a wrapper that records the size of the message that gets written.
func (r *ResponseReverter) Write(buf []byte) (int, error) {
	n, err := r.ResponseWriter.Write(buf)
	return n, err
}
