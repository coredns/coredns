package bind

import (
	"fmt"
	"net"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/mholt/caddy"
)

func setupBind(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)

	// addresses will be consolidated over all BIND directives available in that BlocServer
	allAddresses := []string{}
	for c.Next() {
		addresses := c.RemainingArgs()
		if len(addresses) == 0 {
			return plugin.Error("bind", fmt.Errorf("at least one address is expected"))
		}
		for _, addr := range addresses {
			if net.ParseIP(addr) == nil {
				return plugin.Error("bind", fmt.Errorf("not a valid IP address: %s", addr))
			}
		}
		allAddresses = append(allAddresses, addresses...)
	}
	config.ListenHosts = allAddresses
	return nil
}
