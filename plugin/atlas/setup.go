package atlas

import (
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
	for c.Next() {
		args := c.RemainingArgs()
		log.Infof("controller: %+v => %v\n", c, args)

	}

	//if c.NextArg() {	//	// If there was another token, return an error, because we don't have any configuration.
	//	// Any errors returned from this setup function should be wrapped with plugin.Error, so we
	//	// can present a slightly nicer error message to the user.
	//	return plugin.Error(plgName, c.ArgErr())
	//}

	// Add the Plugin to CoreDNS, so Servers can use it in their plugin chain.
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {

		return Atlas{Next: next}
	})

	// All OK, return a nil error.
	return nil
}
