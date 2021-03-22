// Package rewrite is a plugin for rewriting requests internally to something different.
package rewrite

import (
	"context"
	"fmt"
	"strings"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// possible actions for the flags
const (
	setAction   = "set"
	clearAction = "clear"
)

// current supported flags
const (
	authoritative      = "aa"
	recursionAvailable = "ra"
	recursionDesired   = "rd"
)

type flagRule struct {
	action     string
	flag       string
	nextAction string
}

func newFlagRule(nextAction string, args ...string) (Rule, error) {
	action := strings.ToLower(args[0])
	if action != setAction && action != clearAction {
		return nil, fmt.Errorf("invalid action: %s", action)
	}

	flag := strings.ToLower(args[1])
	switch flag {
	case authoritative:
	case recursionAvailable:
	case recursionDesired:
	default:
		return nil, fmt.Errorf("invalid flag: %s", flag)
	}
	return &flagRule{
		action:     action,
		flag:       flag,
		nextAction: nextAction,
	}, nil
}

func (rule *flagRule) Rewrite(ctx context.Context, state request.Request) Result { return RewriteDone }

func (rule *flagRule) Mode() string { return rule.nextAction }

func (rule *flagRule) GetResponseRules() []ResponseRule {
	return []ResponseRule{
		{
			Active: true,
			Type:   "flag",
			Flag:   rule,
		},
	}
}

func (rule *flagRule) rewriteFlag(dns *dns.Msg) {
	actionValue := rule.action == setAction
	if rule.flag == authoritative && dns.Authoritative != actionValue {
		dns.Authoritative = actionValue
	}
	if rule.flag == recursionAvailable && dns.RecursionAvailable != actionValue {
		dns.RecursionAvailable = actionValue
	}
	if rule.flag == recursionDesired && dns.RecursionDesired != actionValue {
		dns.RecursionDesired = actionValue
	}
}
