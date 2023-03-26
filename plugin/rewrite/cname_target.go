package rewrite

import (
	"context"
	"fmt"
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

// cNameResponseRule is cname target rewrite rule.
type cnameResponseRule struct {
	fromTarget string
	toTarget   string
	nextAction string
	state      request.Request
	ctx        context.Context
	Upstream   UpstreamInt // Upstream for looking up external names during the resolution process.
}

func (r *cnameResponseRule) RewriteResponse(res *dns.Msg, rr dns.RR) {
	// logic to rewrite the cname target of dns response
	switch rr.Header().Rrtype {
	case dns.TypeCNAME:
		// rename the target of the cname response
		if cname, ok := rr.(*dns.CNAME); ok {
			if cname.Target == r.fromTarget {
				// create upstream request to get the A record for the new target
				r.state.Req.Question[0].Name = r.toTarget
				upRes, err := r.Upstream.Lookup(r.ctx, r.state, r.toTarget, dns.TypeA)

				if err != nil {
					log.Infof("Error upstream request %v", err)
				}

				var newAnswer []dns.RR
				// iterate over first upstram response
				// add the cname record to the new answer
				for _, rr := range res.Answer {
					if cname, ok := rr.(*dns.CNAME); ok {
						// change the target name in the response
						cname.Target = r.toTarget
						newAnswer = append(newAnswer, rr)
					}
				}
				// iterate over upstream response made
				for _, rr := range upRes.Answer {
					if rr.Header().Name == r.toTarget {
						newAnswer = append(newAnswer, rr)
					}
				}
				res.Answer = newAnswer
			}
		}
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
	rule := cnameResponseRule{
		fromTarget: fromTarget,
		toTarget:   toTarget,
		nextAction: nextAction,
		Upstream:   upstream.New(),
	}
	log.Infof("cname rule created rule data %v'", rule)
	return &rule, nil
}

// Rewrite rewrites the current request.
func (rule *cnameResponseRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	if len(rule.fromTarget) > 0 && len(rule.toTarget) > 0 {
		rule.state = state
		rule.ctx = ctx
		return ResponseRules{rule}, RewriteDone
	}
	return nil, RewriteIgnored
}

// Mode returns the processing mode.
func (rule *cnameResponseRule) Mode() string { return rule.nextAction }
