package view

import (
	"github.com/Knetic/govaluate"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/expression"
)

type View struct {
	rules      []*govaluate.EvaluableExpression
	extractors expression.ExtractorMap
	Next       plugin.Handler
}

