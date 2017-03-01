package rewrite

import (
	"fmt"

	"github.com/coredns/coredns/middleware"

	"github.com/miekg/dns"
)

type nameRule struct {
	From, To string
}

func newNameRule(args ...string) (Rule, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("Name rules must have exactly two arguments")
	}
	return &nameRule{middleware.Name(args[0]).Normalize(), middleware.Name(args[1]).Normalize()}, nil
}

// Rewrite rewrites the the current request.
func (rule *nameRule) Rewrite(r *dns.Msg) Result {
	if rule.From == r.Question[0].Name {
		r.Question[0].Name = rule.To
		return RewriteDone
	}
	return RewriteIgnored
}
