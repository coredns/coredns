package dnslkg

import (
	"regexp"
	"strconv"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("dnslkg", setup) }

func setup(c *caddy.Controller) error {
	d, err := parse(c)
	if err != nil {
		return plugin.Error("dnslkg", err)
	}

	d.store = newMemStore(d.maxEntries)

	c.OnShutdown(func() error { return d.store.Close() })

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		d.Next = next
		return d
	})

	return nil
}

// parse builds a *DnsLKG from the Corefile configuration.
//
//	dnslkg {
//	    max_entries N
//	    ttl         DURATION
//	    include     REGEX...
//	    exclude     REGEX...
//	}
func parse(c *caddy.Controller) (*DnsLKG, error) {
	d := &DnsLKG{
		ttl: defaultTTL,
	}

	if !c.Next() { // 'dnslkg'
		return nil, c.ArgErr()
	}

	if args := c.RemainingArgs(); len(args) > 0 {
		return nil, c.ArgErr()
	}

	for c.NextBlock() {
		switch c.Val() {
		case "max_entries":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			n, err := strconv.Atoi(args[0])
			if err != nil {
				return nil, c.Errf("invalid max_entries %q: %v", args[0], err)
			}
			if n <= 0 {
				return nil, c.Errf("max_entries must be positive: %q", args[0])
			}
			d.maxEntries = n
		case "ttl":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			dur, err := time.ParseDuration(args[0])
			if err != nil {
				return nil, c.Errf("invalid ttl %q: %v", args[0], err)
			}
			if dur < 0 {
				return nil, c.Errf("ttl cannot be negative: %q", args[0])
			}
			d.ttl = dur
		case "include":
			res, err := compilePatterns(c)
			if err != nil {
				return nil, err
			}
			d.include = append(d.include, res...)
		case "exclude":
			res, err := compilePatterns(c)
			if err != nil {
				return nil, err
			}
			d.exclude = append(d.exclude, res...)
		default:
			return nil, c.Errf("unknown property %q", c.Val())
		}
	}

	return d, nil
}

// compilePatterns reads and compiles the regular expressions following an
// include/exclude directive.
func compilePatterns(c *caddy.Controller) ([]*regexp.Regexp, error) {
	args := c.RemainingArgs()
	if len(args) == 0 {
		return nil, c.ArgErr()
	}
	res := make([]*regexp.Regexp, 0, len(args))
	for _, p := range args {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, c.Errf("invalid regex %q: %v", p, err)
		}
		res = append(res, re)
	}
	return res, nil
}
