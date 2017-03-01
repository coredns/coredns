package rewrite

import (
	"fmt"
	"strings"

	"github.com/miekg/dns"
)

type classRule struct {
	fromClass, toClass uint16
}

func newClassRule(args ...string) (Rule, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("Class rules must have exactly two arguments")
	}
	var from, to uint16
	var ok bool
	if from, ok = dns.StringToClass[strings.ToUpper(args[0])]; !ok {
		return nil, fmt.Errorf("Invalid class '%s'", strings.ToUpper(args[0]))
	}
	if to, ok = dns.StringToClass[strings.ToUpper(args[1])]; !ok {
		return nil, fmt.Errorf("Invalid class '%s'", strings.ToUpper(args[1]))
	}
	return &classRule{fromClass: from, toClass: to}, nil
}

// Rewrite rewrites the the current request.
func (rule *classRule) Rewrite(r *dns.Msg) Result {
	if rule.fromClass > 0 && rule.toClass > 0 {
		if r.Question[0].Qclass == rule.fromClass {
			r.Question[0].Qclass = rule.toClass
			return RewriteDone
		}
	}
	return RewriteIgnored
}
