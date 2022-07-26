package view

import (
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/expression"

	"github.com/Knetic/govaluate"
)

// View is a plugin that enables the creation of expression based advanced routing
type View struct {
	rules      []*govaluate.EvaluableExpression
	extractors expression.ExtractorMap
	Next       plugin.Handler
}
