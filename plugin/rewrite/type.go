package rewrite

import (
	"context"
	"fmt"
	"strings"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// typeRule is a type rewrite rule.
type typeRule struct {
	fromType   uint16
	toType     uint16
	nextAction string
}

func newTypeRule(nextAction string, args ...string) (Rule, error) {
	var from, to uint16
	var ok bool
	if from, ok = dns.StringToType[strings.ToUpper(args[0])]; !ok {
		return nil, fmt.Errorf("invalid type %q", strings.ToUpper(args[0]))
	}
	if to, ok = dns.StringToType[strings.ToUpper(args[1])]; !ok {
		return nil, fmt.Errorf("invalid type %q", strings.ToUpper(args[1]))
	}
	return &typeRule{from, to, nextAction}, nil
}

// Rewrite rewrites the current request.
func (rule *typeRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	if rule.fromType > 0 && rule.toType > 0 {
		if state.QType() == rule.fromType {
			state.Req.Question[0].Qtype = rule.toType
			return nil, RewriteDone
		}
	}
	return nil, RewriteIgnored
}

// Mode returns the processing mode.
func (rule *typeRule) Mode() string { return rule.nextAction }
