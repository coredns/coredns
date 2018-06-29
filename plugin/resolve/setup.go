package resolve

import (
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin(name(), caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	cnr, err := resolveParse(c)
	if err != nil {
		return plugin.Error(name(), err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		cnr.Next = next
		return cnr
	})

	return nil
}

func resolveParse(c *caddy.Controller) (Resolve, error) {
	cnr := Resolve{
		DoCNAME: true,
		DoSRV:   true,
	}

	i := 0
	for c.Next() {
		if i > 0 {
			return cnr, plugin.ErrOnce
		}
		i++

		cnr.Zones = c.RemainingArgs()
		if len(cnr.Zones) == 0 {
			cnr.Zones = make([]string, len(c.ServerBlockKeys))
			copy(cnr.Zones, c.ServerBlockKeys)
		}
		for i, str := range cnr.Zones {
			cnr.Zones[i] = plugin.Host(str).Normalize()
		}
		if c.NextBlock() {
			for {
				switch c.Val() {
				case "no":
					args := c.RemainingArgs()
					if len(args) != 1 {
						return cnr, c.ArgErr()
					}
					if args[0] == "cname" {
						cnr.DoCNAME = false
					}
					if args[0] == "srv" {
						cnr.DoSRV = false
					}
				case "cname":
					args := c.RemainingArgs()
					if len(args) != 0 {
						return cnr, c.ArgErr()
					}
					cnr.DoCNAME = true
				case "srv":
					args := c.RemainingArgs()
					if len(args) != 0 {
						return cnr, c.ArgErr()
					}
					cnr.DoSRV = true
				default:
					if c.Val() != "}" {
						return cnr, c.Errf("unknown property '%s'", c.Val())
					}
				}

				if !c.Next() {
					break
				}
			}
		}
	}

	return cnr, nil
}
