package view

import (
	"context"
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/expression"

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

	i := 0
	for c.Next() {
		i++
		if i > 1 {
			return nil, plugin.ErrOnce
		}
		args := c.RemainingArgs()
		if len(args) != 1 {
			return nil, c.ArgErr()
		}
		v.viewName = args[0]

		for c.NextBlock() {
			switch c.Val() {
			case "expr":
				args := c.RemainingArgs()
				prog, err := expr.Compile(strings.Join(args, " "), expr.Env(expression.DefaultEnv(context.Background(), nil)), expr.DisableBuiltin("type"))
				if err != nil {
					return v, err
				}
				v.progs = append(v.progs, prog)
				if err != nil {
					return nil, err
				}
				continue
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}
	return v, nil
}
