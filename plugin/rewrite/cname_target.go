package rewrite

import (
	"context"
	"fmt"
	"strings"

	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

// cNameResponseRule is cname target rewrite rule.
type cnameResponseRule struct {
	fromTarget string
	toTarget   string
	nextAction string
}

func (r *cnameResponseRule) RewriteResponse(rr dns.RR) {
	// logic to rewrite the cname target of dns response
	switch rr.Header().Rrtype {
	case dns.TypeCNAME:
		// rename the target of the cname response
		if cname, ok := rr.(*dns.CNAME); ok {
			if cname.Target == r.fromTarget {
				cname.Target = r.toTarget
			}
		}
	case dns.TypeA:
		// remove all exising A records and pack new A records got for toTarget name
	}
}

func newCNAMERule(nextAction string, args ...string) (Rule, error) {
	var fromTarget, toTarget string
	// TODO: validations
	if len(args) == 2 {
		fromTarget, toTarget = strings.ToLower(args[0]), strings.ToLower(args[1])
	} else {
		return nil, fmt.Errorf("too few (%d) arguments for a cname rule", len(args))
	}
	rule := cnameResponseRule{fromTarget, toTarget, nextAction}
	log.Infof("cname rule created rule data %v'", rule)
	return &rule, nil
}

// Rewrite rewrites the current request.
func (rule *cnameResponseRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	if len(rule.fromTarget) > 0 && len(rule.toTarget) > 0 {
		// Create rule only if question is A record
		if state.QType() == dns.TypeA {
			return ResponseRules{rule}, RewriteDone
		}
	}
	return nil, RewriteIgnored
}

// Mode returns the processing mode.
func (rule *cnameResponseRule) Mode() string { return rule.nextAction }
