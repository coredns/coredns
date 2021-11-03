package tsig

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

func init() {
	caddy.RegisterPlugin(pluginName, caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	t, err := parse(c)
	if err != nil {
		return plugin.Error(pluginName, c.ArgErr())
	}

	config := dnsserver.GetConfig(c)

	config.TsigSecret = t.secrets

	config.AddPlugin(func(next plugin.Handler) plugin.Handler {
		t.Next = next
		return t
	})

	return nil
}

func parse(c *caddy.Controller) (*TSIGServer, error) {
	t := &TSIGServer{
		secrets: make(map[string]string),
		types:   defaultQTypes,
	}

	for i := 0; c.Next(); i++ {
		if i > 0 {
			return nil, plugin.ErrOnce
		}

		t.Zones = plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys)
		for c.NextBlock() {
			switch c.Val() {
			case "secret":
				args := c.RemainingArgs()
				if len(args) != 2 {
					return nil, c.ArgErr()
				}
				t.secrets[args[0]] = args[1]
			case "require":
				t.types = qTypes{}
				args := c.RemainingArgs()
				if len(args) == 0 {
					return nil, c.ArgErr()
				}
				if args[0] == "all" {
					t.all = true
					continue
				}
				if args[0] == "none" {
					continue
				}
				for _, str := range args {
					qt, ok := dns.StringToType[str]
					if !ok {
						return nil, c.Errf("unknown query type '%s'", str)
					}
					t.types[qt] = struct{}{}
				}
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}

	return t, nil
}

var defaultQTypes = qTypes{}