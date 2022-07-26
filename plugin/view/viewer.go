package view

import (
	"context"

	"github.com/coredns/coredns/plugin/pkg/expression"
	"github.com/coredns/coredns/request"
)

// Filter implements dnsserver.Viewer.  It returns true if all View rules evaluate to true for the given state.
func (v *View) Filter(state request.Request) bool {
	// construct a new state extractor for retrieving info from the state
	statex := expression.NewStateExtractor(context.TODO(), state, v.extractors)

	// return true if all expressions evaluate to true
	for _, expr := range v.rules {
		// evaluate the expression using the state extractor
		result, err := expr.Eval(statex)
		if err != nil {
			return false
		}
		if b, ok := result.(bool); ok && b {
			continue
		}
		// anything other than a boolean true result is considered false
		return false
	}
	return true
}
