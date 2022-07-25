package view

import (
	"strings"

	"github.com/Knetic/govaluate"
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/expression"
)

func init() { plugin.Register("view", setup) }

func setup(c *caddy.Controller) error {
	cond, err := parse(c)
	if err != nil {
		return plugin.Error("view", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		cond.Next = next
		return cond
	})

	return nil
}

func parse(c *caddy.Controller) (*View, error) {
	 v := new(View)

	v.extractors = expression.MakeExtractors()
	funcs := expression.MakeFunctions()

	for c.Next() {
		args := c.RemainingArgs()
		expr, err := govaluate.NewEvaluableExpressionWithFunctions(strings.Join(args, " "), funcs)
		if err != nil {
			return v, err
		}
		v.rules = append(v.rules, expr)
		if err != nil {
			return nil, err
		}
	}
	return v, nil
}

