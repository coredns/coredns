package header

import (
	"fmt"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("header", setup) }

func setup(c *caddy.Controller) error {
	rules, err := parse(c)
	if err != nil {
		return plugin.Error("header", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return Header{
			Rules: rules,
			Next:  next,
		}
	})

	return nil
}

func parse(c *caddy.Controller) ([]Rule, error) {
	for c.Next() {
		var all []Rule
		for c.NextBlock() {
			v := c.Val()
			args := c.RemainingArgs()
			// set up rules
			rules, err := newRules(v, args)
			if err != nil {
				return nil, fmt.Errorf("seting up rule: %w", err)
			}
			all = append(all, rules...)
		}

		// return combined rules
		if len(all) > 0 {
			return all, nil
		}
	}
	return nil, c.ArgErr()

}
