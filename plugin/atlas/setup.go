package atlas

import (
	"fmt"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

// Define log to be a logger with the plugin name in it. This way we can just use log.Info and
// friends to log.
var log = clog.NewWithPlugin(plgName)

const plgName = "atlas"

// init registers this plugin.
func init() {
	plugin.Register(plgName, setup)
}

// setup is the function that gets called when the config parser see the token "atlas". Setup is responsible
// for parsing any extra options the atlas plugin may have. The first token this function sees is "atlas".
func setup(c *caddy.Controller) error {
	var dsn string
	for c.Next() {
		for c.NextBlock() {
			switch c.Val() {
			case "dsn":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return plugin.Error(plgName, fmt.Errorf("expected only one argument for atlas dsn: got %v", len(args)))
				}
				dsn = args[0]
			default:
				log.Infof("default: %+v\n", c.Val())
				return plugin.Error(plgName, c.ArgErr())
			}
		}
	}

	// Add the Plugin to CoreDNS, so Servers can use it in their plugin chain.
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {

		return Atlas{Next: next, Dsn: dsn}
	})

	// All OK, return a nil error.
	return nil
}
