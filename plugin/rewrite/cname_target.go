package rewrite

import (
	"context"
	"fmt"
	"net"
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
				cname.Target = r.toTarget
			}
		}
	case dns.TypeA:
		// TODO: Get the new answer from upstream call, replace the A records in the response
		// remove all exising A records and pack new A records got for toTarget name
		log.Infof("Sending upstream request...")
		resp, err := r.Upstream.Lookup(r.ctx, r.state, r.toTarget, dns.TypeA)
		if err == nil {
			log.Infof("Error upstream request %v", resp)
		} else {
			log.Infof("Error upstream request %v", err)
		}

		if a, ok := rr.(*dns.A); ok {
			a.Header().Name = r.toTarget
			a.A = net.IPv4(124, 43, 66, 43) // *targetIP
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
		// Create rule only if question is A record
		rule.state = state
		rule.ctx = ctx
		if state.QType() == dns.TypeA {
			return ResponseRules{rule}, RewriteDone
		}
	}
	return nil, RewriteIgnored
}

// Mode returns the processing mode.
func (rule *cnameResponseRule) Mode() string { return rule.nextAction }
