/*
package deprecated is used when we deprecated plugin. In plugin.cfg just go from

startup:github.com/mholt/caddy/startupshutdown

To:

startup:deprecated

And things should work as expected. This means starting CoreDNS will fail with an error. We can only
point to the release notes and what are the next steps to take
*/
package deprecated

import (
	"errors"

	"github.com/coredns/coredns/plugin"

	"github.com/mholt/caddy"
)

var removed = []string{"startup", "shutdown"}

func setup(c *caddy.Controller) error {
	c.Next()
	x := c.Val()
	return plugin.Error(x, errors.New("this plugin has been deprecated"))
}

func init() {
	for _, plugin := range removed {
		caddy.RegisterPlugin(plugin, caddy.Plugin{
			ServerType: "dns",
			Action:     setup,
		})
	}
}
