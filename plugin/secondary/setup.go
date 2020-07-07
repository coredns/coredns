package secondary

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/parse"
	"github.com/coredns/coredns/plugin/pkg/upstream"
)

func init() { plugin.Register("secondary", setup) }

func setup(c *caddy.Controller) error {
	zones, err := secondaryParse(c)
	if err != nil {
		return plugin.Error("secondary", err)
	}

	// Add startup functions to retrieve the zone and keep it up to date.
	for _, n := range zones.Names {
		z := zones.Z[n]
		if len(z.TransferFrom) > 0 {
			c.OnStartup(func() error {
				z.StartupOnce.Do(func() {
					go func() {
						z.TransferIn()
						z.Update()
					}()
				})
				return nil
			})
		}
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return Secondary{file.File{Next: next, Zones: zones}}
	})

	return nil
}

func secondaryParse(c *caddy.Controller) (file.Zones, error) {
	z := make(map[string]*file.Zone)
	names := []string{}
	for c.Next() {

		if c.Val() == "secondary" {
			// secondary [origin]
			origins := make([]string, len(c.ServerBlockKeys))
			copy(origins, c.ServerBlockKeys)
			args := c.RemainingArgs()
			if len(args) > 0 {
				origins = args
			}
			for i := range origins {
				origins[i] = plugin.Host(origins[i]).Normalize()
				z[origins[i]] = file.NewZone(origins[i], "stdin")
				names = append(names, origins[i])
			}

			for c.NextBlock() {

				f := []string{}

				switch c.Val() {
				case "transfer":
					var err error
					f, err = parse.TransferIn(c)
					if err != nil {
						return file.Zones{}, err
					}
				default:
					return file.Zones{}, c.Errf("unknown property '%s'", c.Val())
				}

				for _, origin := range origins {
					if f != nil {
						z[origin].TransferFrom = append(z[origin].TransferFrom, f...)
					}
					z[origin].Upstream = upstream.New()
				}
			}
		}
	}
	return file.Zones{Z: z, Names: names}, nil
}
