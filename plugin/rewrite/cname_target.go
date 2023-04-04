package rewrite

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// UpstreamInt wraps the Upstream API for dependency injection during testing
type UpstreamInt interface {
	Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error)
}

// RewriteType is the type of cname target rewrite rules
type RewriteType string

const (
	// CNameExactMatch is the type for exact match of cname target
	CNameExactMatch RewriteType = "exact"
	// CNamePrefixMatch is the type for prefix match of cname target
	CNamePrefixMatch RewriteType = "prefix"
	// CNameSuffixMatch is the type for suffix match of cname target
	CNameSuffixMatch RewriteType = "suffix"
	// CNameSubstringMatch is the type for substring match of cname target
	CNameSubstringMatch RewriteType = "substring"
	// CNameRegexMatch is the type for regex match of cname target
	CNameRegexMatch RewriteType = "regex"
)

// cnameTargetRule is cname target rewrite rule.
type cnameTargetRule struct {
	rewriteType     RewriteType
	paramFromTarget string
	paramToTarget   string
	nextAction      string
	state           request.Request
	ctx             context.Context
	Upstream        UpstreamInt // Upstream for looking up external names during the resolution process.
}

func (r *cnameTargetRule) getFromAndToTarget(inputCName string) (from string, to string) {
	switch r.rewriteType {
	case CNameExactMatch:
		return r.paramFromTarget, r.paramToTarget
	case CNamePrefixMatch:
		if strings.HasPrefix(inputCName, r.paramFromTarget) {
			return inputCName, r.paramToTarget + strings.TrimPrefix(inputCName, r.paramFromTarget)
		}
	case CNameSuffixMatch:
		if strings.HasSuffix(inputCName, r.paramFromTarget) {
			return inputCName, strings.TrimSuffix(inputCName, r.paramFromTarget) + r.paramToTarget
		}
	case CNameSubstringMatch:
		if strings.Contains(inputCName, r.paramFromTarget) {
			return inputCName, strings.Replace(inputCName, r.paramFromTarget, r.paramToTarget, -1)
		}
	case CNameRegexMatch:
		pattern := regexp.MustCompile(r.paramFromTarget)
		regexGroups := pattern.FindStringSubmatch(inputCName)
		if len(regexGroups) == 0 {
			return "", ""
		}
		substitution := r.paramToTarget
		for groupIndex, groupValue := range regexGroups {
			groupIndexStr := "{" + strconv.Itoa(groupIndex) + "}"
			substitution = strings.Replace(substitution, groupIndexStr, groupValue, -1)
		}
		return inputCName, substitution
	}
	return "", ""
}

func (r *cnameTargetRule) RewriteResponse(res *dns.Msg, rr dns.RR) {
	// logic to rewrite the cname target of dns response
	switch rr.Header().Rrtype {
	case dns.TypeCNAME:
		// rename the target of the cname response
		if cname, ok := rr.(*dns.CNAME); ok {
			fromTarget, toTarget := r.getFromAndToTarget(cname.Target)
			if cname.Target == fromTarget {
				// create upstream request with the new target with the same qtype
				r.state.Req.Question[0].Name = toTarget
				upRes, err := r.Upstream.Lookup(r.ctx, r.state, toTarget, r.state.Req.Question[0].Qtype)

				if err != nil {
					log.Errorf("Error upstream request %v", err)
				}

				var newAnswer []dns.RR
				// iterate over first upstram response
				// add the cname record to the new answer
				for _, rr := range res.Answer {
					if cname, ok := rr.(*dns.CNAME); ok {
						// change the target name in the response
						cname.Target = toTarget
						newAnswer = append(newAnswer, rr)
					}
				}
				// iterate over upstream response made
				for _, rr := range upRes.Answer {
					if rr.Header().Name == toTarget {
						newAnswer = append(newAnswer, rr)
					}
				}
				res.Answer = newAnswer
			}
		}
	}
}

func newCNAMERule(nextAction string, args ...string) (Rule, error) {
	var rewriteType RewriteType
	var paramFromTarget, paramToTarget string
	if len(args) == 3 {
		rewriteType = (RewriteType)(strings.ToLower(args[0]))
		switch rewriteType {
		case CNameExactMatch:
		case CNamePrefixMatch:
		case CNameSuffixMatch:
		case CNameSubstringMatch:
		case CNameRegexMatch:
		default:
			return nil, fmt.Errorf("unknown cname rewrite type: %s", rewriteType)
		}
		paramFromTarget, paramToTarget = strings.ToLower(args[1]), strings.ToLower(args[2])
	} else {
		return nil, fmt.Errorf("too few (%d) arguments for a cname rule", len(args))
	}
	rule := cnameTargetRule{
		rewriteType:     rewriteType,
		paramFromTarget: paramFromTarget,
		paramToTarget:   paramToTarget,
		nextAction:      nextAction,
		Upstream:        upstream.New(),
	}
	return &rule, nil
}

// Rewrite rewrites the current request.
func (r *cnameTargetRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	if len(r.rewriteType) > 0 && len(r.paramFromTarget) > 0 && len(r.paramToTarget) > 0 {
		r.state = state
		r.ctx = ctx
		return ResponseRules{r}, RewriteDone
	}
	return nil, RewriteIgnored
}

// Mode returns the processing mode.
func (r *cnameTargetRule) Mode() string { return r.nextAction }
