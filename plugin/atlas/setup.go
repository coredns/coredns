package atlas

import (
	"fmt"
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

// Define log to be a logger with the plugin name in it. This way we can just use log.Info and
// friends to log.
var log = clog.NewWithPlugin(plgName)

const plgName = "atlas"

type config struct {
	automigrate bool
	dsn         string
	dsnFile     string
}

// init registers this plugin.
func init() {
	plugin.Register(plgName, setup)
}

// setup is the function that gets called when the config parser see the token "atlas". Setup is responsible
// for parsing any extra options the atlas plugin may have. The first token this function sees is "atlas".
func setup(c *caddy.Controller) error {

	cfg := config{
		automigrate: false,
	}

	for c.Next() {
		for c.NextBlock() {
			switch c.Val() {
			case "dsn":
				args := c.RemainingArgs()
				if len(args) <= 0 {
					return plugin.Error(plgName, fmt.Errorf("atlas: argument for dsn expected"))
				}
				cfg.dsn = args[0]
			case "file":
				args := c.RemainingArgs()
				if len(args) <= 0 {
					return plugin.Error(plgName, fmt.Errorf("atlas: argument for file expected"))
				}
				cfg.dsnFile = args[0]
			case "automigrate":
				var err error
				args := c.RemainingArgs()
				if len(args) <= 0 {
					return plugin.Error(plgName, fmt.Errorf("atlas: argument for automigrate expected"))
				}
				if cfg.automigrate, err = strconv.ParseBool(args[0]); err != nil {
					return err
				}
			default:
				return plugin.Error(plgName, c.ArgErr())
			}
		}
	}

	// Add the Plugin to CoreDNS, so Servers can use it in their plugin chain.
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		fmt.Printf("%+v\n", cfg)
		return Atlas{Next: next, cfg: cfg}
	})

	// All OK, return a nil error.
	return nil
}
