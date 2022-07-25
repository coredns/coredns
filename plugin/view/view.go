package view

import (
	"github.com/Knetic/govaluate"
	"github.com/coredns/coredns/plugin"
)

type View struct {
	rules      []*govaluate.EvaluableExpression
	extractors extractorMap
	Next       plugin.Handler
}

