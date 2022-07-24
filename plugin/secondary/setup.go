package secondary

import (
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/parse"
	"github.com/coredns/coredns/plugin/pkg/upstream"
)

var log = clog.NewWithPlugin("secondary")

func init() { plugin.Register("secondary", setup) }

func setup(c *caddy.Controller) error {
	zones, err := secondaryParse(c)
	if err != nil {
		return plugin.Error("secondary", err)
	}

	// Add startup functions to retrieve the zone and keep it up to date.
	for i := range zones.Names {
		n := zones.Names[i]
		z := zones.Z[n]
		if len(z.TransferFrom) > 0 {
			c.OnStartup(func() error {
				z.StartupOnce.Do(func() {
					go func() {
						dur := time.Millisecond * 250
						step := time.Duration(2)
						max := time.Second * 10
						for {
							err := z.TransferIn()
							if err == nil {
								break
							}
							log.Warningf("All '%s' masters failed to transfer, retrying in %s: %s", n, dur.String(), err)
							time.Sleep(dur)
							dur = step * dur
							if dur > max {
								dur = max
							}
						}
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
			origins := plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys)
			for i := range origins {
				z[origins[i]] = file.NewZone(origins[i], "stdin")
				names = append(names, origins[i])
			}

			for c.NextBlock() {
				var f []string

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
