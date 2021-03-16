package bind

import (
	"fmt"
	"net"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func setup(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)

	// addresses will be consolidated over all BIND directives available in that BlocServer
	all := []string{}
	for c.Next() {
		args := c.RemainingArgs()
		if len(args) == 0 {
			return plugin.Error("bind", fmt.Errorf("at least one address or interface name is expected"))
		}

		ifaces, err := net.Interfaces()
		if err != nil {
			return plugin.Error("bind", fmt.Errorf("failed to get interfaces list"))
		}

		var isIface bool
		for _, arg := range args {
			isIface = false
			for _, iface := range ifaces {
				if arg == iface.Name {
					isIface = true
					addrs, err := iface.Addrs()
					if err != nil {
						return plugin.Error("bind", fmt.Errorf("failed to get the IP(s) of the interface: %s", arg))
					}
					for _, addr := range addrs {
						if ipnet, ok := addr.(*net.IPNet); ok {
							all = append(all, ipnet.IP.String())
						}
					}
				}
			}
			if !isIface {
				if net.ParseIP(arg) == nil {
					return plugin.Error("bind", fmt.Errorf("not a valid IP address: %s", arg))
				}
				all = append(all, arg)
			}
		}
	}
	config.ListenHosts = all
	return nil
}
