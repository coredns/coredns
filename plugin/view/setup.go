package view

import (
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/expression"
	"github.com/coredns/coredns/request"

	"github.com/antonmedv/expr"
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

	for c.Next() {
		args := c.RemainingArgs()

		prog, err := expr.Compile(strings.Join(args, " "), expr.Env(expression.DefaultEnv(&request.Request{})))
		if err != nil {
			return v, err
		}
		v.progs = append(v.progs, prog)
		if err != nil {
			return nil, err
		}
	}
	return v, nil
}
