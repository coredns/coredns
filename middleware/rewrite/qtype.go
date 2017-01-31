// Package rewrite is middleware for rewriting requests internally to something different.
package rewrite

import (
	"strings"

	"github.com/miekg/dns"
)

// QtypeRule is a type rewrite rule.
type QtypeRule struct {
	fromType, toType uint16
}

// Initializer
func (rule QtypeRule) New(args ...string) Rule {
	from, to := args[0], strings.Join(args[1:], " ")
	return &QtypeRule{dns.StringToType[from], dns.StringToType[to]}
}

// Rewrite rewrites the the current request.
func (rule QtypeRule) Rewrite(r *dns.Msg) Result {
	if rule.fromType > 0 && rule.toType > 0 {
		if r.Question[0].Qtype == rule.fromType {
			r.Question[0].Qtype = rule.toType
			return RewriteDone
		}
	}
	return RewriteIgnored
}
