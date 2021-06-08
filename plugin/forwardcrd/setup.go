package forwardcrd

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
)

func init() {
	plugin.Register("forwardcrd", setup)
}

func setup(c *caddy.Controller) error {
	return nil
}
