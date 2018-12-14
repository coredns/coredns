package erratic

import (
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	cs "github.com/coredns/coredns/plugin/pkg/coresetup"

	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("erratic", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	e, err := parseErratic(c)
	if err != nil {
		return plugin.Error("erratic", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return e
	})

	return nil
}

func parseErratic(c *caddy.Controller) (*Erratic, error) {
	e := &Erratic{drop: 2}
	drop := false // true if we've seen the drop keyword

	for c.Next() { // 'erratic'
		for c.NextBlock() {
			switch c.Val() {
			case "drop":
				vals, err := cs.Parse(c, cs.Int{0, -1, cs.DefaultInt(2)})
				if err != nil {
					return nil, err
				}
				e.drop = uint64(vals[0].IntValue())
				drop = true
			case "delay":
				vals, err := cs.Parse(c, cs.Int{0, -1, cs.DefaultInt(2)}, cs.Duration{cs.DefaultDuration(100 * time.Millisecond)})
				if err != nil {
					return nil, err
				}
				e.delay = uint64(vals[0].IntValue())
				e.duration = vals[1].DurationValue()
			case "truncate":
				vals, err := cs.Parse(c, cs.Int{0, -1, cs.DefaultInt(0)})
				if err != nil {
					return nil, err
				}
				e.truncate = uint64(vals[0].IntValue())
			case "large":
				e.large = true
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}
	if (e.delay > 0 || e.truncate > 0) && !drop { // delay is set, but we've haven't seen a drop keyword, remove default drop stuff
		e.drop = 0
	}

	return e, nil
}
