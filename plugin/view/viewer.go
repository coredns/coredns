package view

import (
	"github.com/coredns/coredns/plugin/pkg/expression"
	"github.com/coredns/coredns/request"
)

func (v *View) Filter(state request.Request) bool {
	params := expression.MakeParameters(state, v.extractors)
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
