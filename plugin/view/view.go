package view

import (
	"github.com/Knetic/govaluate"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/expression"
)

// View is a plugin that enables the creation of expression based advanced routing
type View struct {
	rules      []*govaluate.EvaluableExpression
	extractors expression.ExtractorMap
	Next       plugin.Handler
}
