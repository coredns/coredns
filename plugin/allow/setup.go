package allow

import (
    "strings"

    "github.com/coredns/coredns/core/dnsserver"
    "github.com/coredns/coredns/plugin"

    "github.com/mholt/caddy"
)

func init() {
    caddy.RegisterPlugin("allow", caddy.Plugin{
        ServerType: "dns",
        Action: setup,
    })
}

func setup(c *caddy.Controller) error {
    cidrs, err := argsParse(c)
    if err != nil {
        return plugin.Error("allow", err)
    }

    dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
	    return Allow{Next: next, Cidrs: cidrs}
    })

    return nil
}

func argsParse(c *caddy.Controller) ([]string, error) {
	var cidrs []string

	for c.Next() {
		args := c.RemainingArgs()
		for _, arg := range args {
			cidrs = append(cidrs, strings.ToLower(arg))
		}
	}

	return cidrs, nil
}
