package rewrite

import (
	"context"
	"fmt"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Result is the result of a rewrite
type Result int

const (
	// RewriteIgnored is returned when rewrite is not done on request.
	RewriteIgnored Result = iota
	// RewriteDone is returned when rewrite is done on request.
	RewriteDone
)

// These are defined processing mode.
const (
	// Processing should stop after completing this rule
	Stop = "stop"
	// Processing should continue to next rule
	Continue = "continue"
)

// Rewrite is a plugin to rewrite requests internally before being handled.
type Rewrite struct {
	Next  plugin.Handler
	Rules []Rule
	RevertPolicy
}

// ServeDNS implements the plugin.Handler interface.
func (rw Rewrite) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if rw.RevertPolicy == nil {
		rw.RevertPolicy = NewRevertPolicy(false, false)
	}
	wr := NewResponseReverter(w, r, rw.RevertPolicy)
	state := request.Request{W: w, Req: r}

	for _, rule := range rw.Rules {
		respRules, result := rule.Rewrite(ctx, state)
		if result == RewriteDone {
			if _, ok := dns.IsDomainName(state.Req.Question[0].Name); !ok {
				err := fmt.Errorf("invalid name after rewrite: %s", state.Req.Question[0].Name)
				state.Req.Question[0] = wr.originalQuestion
				return dns.RcodeServerFailure, err
			}
			wr.ResponseRules = append(wr.ResponseRules, respRules...)
			if rule.Mode() == Stop {
				if !rw.RevertPolicy.DoRevert() {
					return plugin.NextOrFailure(rw.Name(), rw.Next, ctx, w, r)
				}
				rcode, err := plugin.NextOrFailure(rw.Name(), rw.Next, ctx, wr, r)
				if plugin.ClientWrite(rcode) {
					return rcode, err
				}
				// The next plugins didn't write a response, so write one now with the ResponseReverter.
				// If server.ServeDNS does this then it will create an answer mismatch.
				res := new(dns.Msg).SetRcode(r, rcode)
				state.SizeAndDo(res)
				wr.WriteMsg(res)
				// return success, so server does not write a second error response to client
				return dns.RcodeSuccess, err
			}
		}
	}
	if !rw.RevertPolicy.DoRevert() || len(wr.ResponseRules) == 0 {
		return plugin.NextOrFailure(rw.Name(), rw.Next, ctx, w, r)
	}
	return plugin.NextOrFailure(rw.Name(), rw.Next, ctx, wr, r)
}

// Name implements the Handler interface.
func (rw Rewrite) Name() string { return "rewrite" }

// Rule describes a rewrite rule.
type Rule interface {
	// Rewrite rewrites the current request.
	Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result)
	// Mode returns the processing mode stop or continue.
	Mode() string
}

func newRule(args ...string) (Rule, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no rule type specified for rewrite")
	}

	arg0 := strings.ToLower(args[0])
	var ruleType string
	var expectNumArgs, startArg int
	mode := Stop
	switch arg0 {
	case Continue:
		mode = Continue
		if len(args) < 2 {
			return nil, fmt.Errorf("continue rule must begin with a rule type")
		}
		ruleType = strings.ToLower(args[1])
		expectNumArgs = len(args) - 1
		startArg = 2
	case Stop:
		if len(args) < 2 {
			return nil, fmt.Errorf("stop rule must begin with a rule type")
		}
		ruleType = strings.ToLower(args[1])
		expectNumArgs = len(args) - 1
		startArg = 2
	default:
		// for backward compatibility
		ruleType = arg0
		expectNumArgs = len(args)
		startArg = 1
	}

	switch ruleType {
	case "answer":
		return nil, fmt.Errorf("response rewrites must begin with a name rule")
	case "name":
		return newNameRule(mode, args[startArg:]...)
	case "class":
		if expectNumArgs != 3 {
			return nil, fmt.Errorf("%s rules must have exactly two arguments", ruleType)
		}
		return newClassRule(mode, args[startArg:]...)
	case "type":
		if expectNumArgs != 3 {
			return nil, fmt.Errorf("%s rules must have exactly two arguments", ruleType)
		}
		return newTypeRule(mode, args[startArg:]...)
	case "edns0":
		return newEdns0Rule(mode, args[startArg:]...)
	case "ttl":
		return newTTLRule(mode, args[startArg:]...)
	default:
		return nil, fmt.Errorf("invalid rule type %q", args[0])
	}
}
