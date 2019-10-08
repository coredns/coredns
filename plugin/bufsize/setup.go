package bufsize

import (
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/caddyserver/caddy"
)

func init() { plugin.Register("bufsize", setup) }

func setup(c *caddy.Controller) error {
	bufsize, err := bufsizeParse(c)
	if err != nil {
		return plugin.Error("bufsize", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return Bufsize{Next: next, Size: bufsize}
	})

	return nil
}

func bufsizeParse(c *caddy.Controller) (int, error) {
	// Use 512 byte as the default
	bufsize := 512
	args := c.RemainingArgs()
	bufsize = args

	/*
		for c.Next() {
			if i > 0 {
				return bufsize, plugin.ErrOnce
			}
			i++
			args := c.RemainingArgs()
			bufsize = args
		}
	*/

	return bufsize, nil
}
