package secondary

import (
	"sync"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/catalog"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/parse"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/plugin/transfer"
)

var log = clog.NewWithPlugin("secondary")

func init() { plugin.Register("secondary", setup) }

func setup(c *caddy.Controller) error {
	zones, fall, catalogZones, err := secondaryParse(c)
	if err != nil {
		return plugin.Error("secondary", err)
	}

	s := newSecondary(zones, fall, catalogZones)
	var x *transfer.Transfer
	c.OnStartup(func() error {
		t := dnsserver.GetConfig(c).Handler("transfer")
		if t != nil {
			x = t.(*transfer.Transfer)
			s.Xfer = x // if found this must be OK.
		}
		return nil
	})

	// Add startup functions to retrieve the zone and keep it up to date.
	for i := range zones.Names {
		n := zones.Names[i]
		z := zones.Z[n]
		if len(z.TransferFrom) > 0 {
			// In order to support secondary plugin reloading.
			updateShutdown := make(chan bool)
			var updateShutdownOnce sync.Once

			c.OnStartup(func() error {
				z.StartupOnce.Do(func() {
					go s.transferAndUpdate(n, z, x, updateShutdown)
				})
				return nil
			})
			c.OnShutdown(func() error {
				updateShutdownOnce.Do(func() { close(updateShutdown) })
				return nil
			})
		}
	}
	c.OnShutdown(func() error {
		s.stopDynamicZones()
		return nil
	})

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		s.Next = next
		return s
	})

	return nil
}

func newSecondary(zones file.Zones, fall fall.F, catalogZones map[string]struct{}) *Secondary {
	s := &Secondary{
		File:               file.File{Zones: zones, Fall: fall},
		zoneNames:          make(map[*file.Zone]string, len(zones.Z)),
		dynamicZones:       make(map[string]*dynamicZone),
		catalogs:           make(map[string]*catalog.Catalog),
		catalogZones:       catalogZones,
		catalogMemberZones: make(map[string]map[string]struct{}),
	}
	for name, zone := range zones.Z {
		s.zoneNames[zone] = name
	}
	s.ZoneLookupFunc = s.lookupZone
	s.TransferInFunc = func(z *file.Zone, t *transfer.Transfer) error {
		return s.transferIn(s.zoneName(z), z, t)
	}
	return s
}

func (s *Secondary) transferAndUpdate(origin string, z *file.Zone, x *transfer.Transfer, updateShutdown chan bool) {
	dur := time.Millisecond * 250
	max := time.Second * 10
	for {
		err := s.transferIn(origin, z, x)
		if err == nil {
			break
		}
		log.Warningf("All '%s' masters failed to transfer, retrying in %s: %s", origin, dur.String(), err)
		if waitForTransferRetry(updateShutdown, dur) {
			return
		}
		dur <<= 1 // double the duration
		if dur > max {
			dur = max
		}
	}
	select {
	case <-updateShutdown:
		return
	default:
	}
	z.UpdateWithTransfer(updateShutdown, x, func(z *file.Zone, t *transfer.Transfer) error {
		return s.transferIn(origin, z, t)
	})
}

func waitForTransferRetry(updateShutdown <-chan bool, dur time.Duration) bool {
	timer := time.NewTimer(dur)
	defer timer.Stop()
	select {
	case <-timer.C:
		return false
	case <-updateShutdown:
		return true
	}
}

func secondaryParse(c *caddy.Controller) (file.Zones, fall.F, map[string]struct{}, error) {
	z := make(map[string]*file.Zone)
	names := []string{}
	fall := fall.F{}
	catalogZones := map[string]struct{}{}
	for c.Next() {
		if c.Val() == "secondary" {
			// secondary [origin]
			origins := plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys)
			for i := range origins {
				z[origins[i]] = file.NewZone(origins[i], "stdin")
				names = append(names, origins[i])
			}

			hasTransfer := false
			for c.NextBlock() {
				var f []string

				switch c.Val() {
				case "transfer":
					var err error
					f, err = parse.TransferIn(c)
					if err != nil {
						return file.Zones{}, fall, nil, err
					}
					hasTransfer = true
				case "catalog":
					if len(c.RemainingArgs()) != 0 {
						return file.Zones{}, fall, nil, c.ArgErr()
					}
					for _, origin := range origins {
						catalogZones[origin] = struct{}{}
					}
				case "fallthrough":
					fall.SetZonesFromArgs(c.RemainingArgs())
				default:
					return file.Zones{}, fall, nil, c.Errf("unknown property '%s'", c.Val())
				}

				for _, origin := range origins {
					if f != nil {
						z[origin].TransferFrom = append(z[origin].TransferFrom, f...)
					}
					z[origin].Upstream = upstream.New()
				}
			}
			if !hasTransfer {
				return file.Zones{}, fall, nil, c.Err("secondary zones require a transfer from property")
			}
		}
	}
	return file.Zones{Z: z, Names: names}, fall, catalogZones, nil
}
