package view

import (
	"github.com/coredns/coredns/request"
)

func (v *View) Filter(state request.Request) bool {
	// TODO: Context need to be passed to Filter, otherwise metadata eval cannot work.  Requires interface change.
	// but the context, at the time of evaluating the filter funcs, wont have metadata in it yet ... hmmm ...
	// .. so dnsserver ServeDNS will need to peek into each chain to extract metadata. ugh
	params := Parameters{state: &state, extractors: v.extractors}
	// return true if all expressions evaluate to true
	for _, expr := range v.rules {
		result, err := expr.Eval(params)
		if err != nil {
			return false
		}
		if b, ok := result.(bool); ok && b {
			continue
		}
		return false
	}
	return true
}
