// Package rewrite is middleware for rewriting requests internally to something different.
package rewrite

import (
	"fmt"
	"strings"

	"github.com/miekg/dns"
)

// typeRule is a type rewrite rule.
type typeRule struct {
	fromType, toType uint16
}

func newTypeRule(args ...string) (Rule, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("Type rules must have exactly two arguments")
	}
	var from, to uint16
	var ok bool
	if from, ok = dns.StringToType[strings.ToUpper(args[0])]; !ok {
		return nil, fmt.Errorf("Invalid class '%s'", strings.ToUpper(args[0]))
	}
	if to, ok = dns.StringToType[strings.ToUpper(args[1])]; !ok {
		return nil, fmt.Errorf("Invalid class '%s'", strings.ToUpper(args[1]))
	}
	return &typeRule{fromType: from, toType: to}, nil
}

// Rewrite rewrites the the current request.
func (rule *typeRule) Rewrite(r *dns.Msg) Result {
	if rule.fromType > 0 && rule.toType > 0 {
		if r.Question[0].Qtype == rule.fromType {
			r.Question[0].Qtype = rule.toType
			return RewriteDone
		}
	}
	return RewriteIgnored
}
