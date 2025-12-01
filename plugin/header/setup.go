package header

import (
	"fmt"
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("header", setup) }

func setup(c *caddy.Controller) error {
	queryRules, responseRules, err := parse(c)
	if err != nil {
		return plugin.Error("header", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return Header{
			QueryRules:    queryRules,
			ResponseRules: responseRules,
			Next:          next,
		}
	})

	return nil
}

func parse(c *caddy.Controller) ([]Rule, []Rule, error) {
	for c.Next() {
		var queryRules []Rule
		var responseRules []Rule

		for c.NextBlock() {
			selector := strings.ToLower(c.Val())

			var action string
			if selector == "set" || selector == "clear" {
				log.Warningf("The selector for header rule in line %d isn't explicit defined. "+
					"Assume rule applies for selector 'response'. This syntax is deprecated. "+
					"In future versions of CoreDNS the selector must be explicit defined.",
					c.Line())

				action = selector
				selector = "response"
			} else if selector == "query" || selector == "response" {
				if c.NextArg() {
					action = c.Val()
				}
			} else {
				return nil, nil, fmt.Errorf("setting up rule: invalid selector=%s should be query or response", selector)
			}

			args := c.RemainingArgs()
			rules, err := newRules(action, args)
			if err != nil {
				return nil, nil, fmt.Errorf("setting up rule: %w", err)
			}

			if selector == "response" {
				responseRules = append(responseRules, rules...)
			} else {
				queryRules = append(queryRules, rules...)
			}
		}

		if len(queryRules) > 0 || len(responseRules) > 0 {
			return queryRules, responseRules, nil
		}
	}
	return nil, nil, c.ArgErr()
}
