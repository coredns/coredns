package recursion

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("recursion", setup) }

func setup(c *caddy.Controller) error {
	rec := New()
	err := recursionParse(c, rec)
	if err != nil {
		return plugin.Error("recursion", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		rec.Next = next
		return rec
	})

	return nil
}

func recursionParse(c *caddy.Controller, f *Recursion) error {
	j := 0
	for c.Next() {
		if j > 0 {
			return plugin.ErrOnce
		}
		j++

		// Refinements? In an extra block.
		for c.NextBlock() {
			switch c.Val() {
			case "except":
				ignore := c.RemainingArgs()
				if len(ignore) == 0 {
					return c.ArgErr()
				}
				for i := 0; i < len(ignore); i++ {
					f.ignored = append(f.ignored, plugin.Host(ignore[i]).NormalizeExact()...)
				}

			case "timeout":
				if !c.NextArg() {
					return c.ArgErr()
				}
				dur, err := time.ParseDuration(c.Val())
				if err != nil {
					return err
				}
				if dur < 0 {
					return fmt.Errorf("timeout can't be negative: %s", dur)
				}
				f.timeout = dur

			case "max_tries":
				if !c.NextArg() {
					return c.ArgErr()
				}
				n, err := strconv.ParseUint(c.Val(), 10, 32)
				if err != nil {
					return err
				}
				f.maxTries = uint32(n)

			case "max_depth":
				if !c.NextArg() {
					return c.ArgErr()
				}
				n, err := strconv.ParseUint(c.Val(), 10, 32)
				if err != nil {
					return err
				}
				f.maxDepth = uint32(n)

			case "max_concurrent":
				if !c.NextArg() {
					return c.ArgErr()
				}
				n, err := strconv.Atoi(c.Val())
				if err != nil {
					return err
				}
				if n < 0 {
					return fmt.Errorf("max_concurrent can't be negative: %d", n)
				}
				f.ErrLimitExceeded = errors.New("concurrent queries exceeded maximum " + c.Val())
				f.maxConcurrent = int64(n)

			default:
				return c.Errf("unknown property '%s'", c.Val())
			}
		}
	}
	return nil
}
