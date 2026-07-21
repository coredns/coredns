package dnslkg

import (
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

	d.store = newMemStore(d.maxEntries, d.maxAge)

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
//	    fallback_on      nxdomain nodata timeout error
//	    max_age          DURATION
//	    fallback_timeout DURATION
//	    ttl              DURATION
//	    max_entries      N
//	    include          PATTERN...
//	    exclude          PATTERN...
//	}
func parse(c *caddy.Controller) (*DnsLKG, error) {
	d := &DnsLKG{
		ttl: defaultTTL,
		fb:  allFallbacks(),
	}
	var rules []rule

	if !c.Next() { // 'dnslkg'
		return nil, c.ArgErr()
	}

	if args := c.RemainingArgs(); len(args) > 0 {
		return nil, c.ArgErr()
	}

	for c.NextBlock() {
		switch c.Val() {
		case "fallback_on":
			fb, err := parseFallbackOn(c)
			if err != nil {
				return nil, err
			}
			d.fb = fb
		case "max_age":
			dur, err := parseDuration(c, false)
			if err != nil {
				return nil, err
			}
			d.maxAge = dur
		case "fallback_timeout":
			dur, err := parseDuration(c, false)
			if err != nil {
				return nil, err
			}
			d.fallbackTimeout = dur
		case "ttl":
			dur, err := parseDuration(c, false)
			if err != nil {
				return nil, err
			}
			d.ttl = dur
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
		case "include":
			rs, err := parseRules(c, true)
			if err != nil {
				return nil, err
			}
			rules = append(rules, rs...)
		case "exclude":
			rs, err := parseRules(c, false)
			if err != nil {
				return nil, err
			}
			rules = append(rules, rs...)
		default:
			return nil, c.Errf("unknown property %q", c.Val())
		}
	}

	d.matcher = newNameMatcher(rules)
	return d, nil
}

// parseDuration reads a single Go duration argument. When allowNegative is
// false a negative value is rejected.
func parseDuration(c *caddy.Controller, allowNegative bool) (time.Duration, error) {
	args := c.RemainingArgs()
	if len(args) != 1 {
		return 0, c.ArgErr()
	}
	dur, err := time.ParseDuration(args[0])
	if err != nil {
		return 0, c.Errf("invalid %s %q: %v", c.Val(), args[0], err)
	}
	if !allowNegative && dur < 0 {
		return 0, c.Errf("%s cannot be negative: %q", c.Val(), args[0])
	}
	return dur, nil
}

// parseFallbackOn reads the set of failure triggers that cause an LKG fallback.
func parseFallbackOn(c *caddy.Controller) (fallbackSet, error) {
	args := c.RemainingArgs()
	if len(args) == 0 {
		return fallbackSet{}, c.ArgErr()
	}
	var fb fallbackSet
	for _, a := range args {
		switch a {
		case "all":
			fb = allFallbacks()
		case "none":
			fb = fallbackSet{}
		case "nxdomain":
			fb.nxdomain = true
		case "nodata":
			fb.nodata = true
		case "timeout":
			fb.timeout = true
		case "error", "servfail":
			fb.serverr = true
		default:
			return fallbackSet{}, c.Errf("unknown fallback_on trigger %q", a)
		}
	}
	return fb, nil
}

// parseRules reads the wildcard domain patterns following an include/exclude
// directive and turns them into rules.
func parseRules(c *caddy.Controller, include bool) ([]rule, error) {
	args := c.RemainingArgs()
	if len(args) == 0 {
		return nil, c.ArgErr()
	}
	rules := make([]rule, 0, len(args))
	for _, p := range args {
		r, err := parseRule(p, include)
		if err != nil {
			return nil, c.Errf("invalid pattern: %v", err)
		}
		rules = append(rules, r)
	}
	return rules, nil
}