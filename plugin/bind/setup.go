package bind

import (
	"errors"
	"fmt"
	"net"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/log"

	"k8s.io/utils/strings/slices"
)

func setup(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)
	// addresses will be consolidated over all BIND directives available in that BlocServer
	all := []string{}
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Warning(plugin.Error("bind", fmt.Errorf("failed to get interfaces list, cannot bind by interface name: %s", err)))
	}

	for c.Next() {
		b, err := parse(c)
		if err != nil {
			return plugin.Error("bind", err)
		}

		ips, err := listIP(b.addrs, ifaces)
		if err != nil {
			return plugin.Error("bind", err)
		}

		except, err := listIP(b.except, ifaces)
		if err != nil {
			return plugin.Error("bind", err)
		}

		for _, ip := range ips {
			if !slices.Contains(except, ip) {
				all = append(all, ip)
			}
		}
	}

	config.ListenHosts = all
	return nil
}

func parse(c *caddy.Controller) (*bind, error) {
	b := &bind{}
	b.addrs = c.RemainingArgs()
	if len(b.addrs) == 0 {
		return nil, errors.New("at least one IP address, interface or hostname is expected")
	}
	for c.NextBlock() {
		switch c.Val() {
		case "except":
			b.except = c.RemainingArgs()
			if len(b.except) == 0 {
				return nil, errors.New("at least one IP address, interface or hostname must be given to except subdirective")
			}
		default:
			return nil, fmt.Errorf("invalid option %q", c.Val())
		}
	}
	return b, nil
}

// listIP returns a list of IP addresses from a list of arguments which can be either IP-Address or Interface-Name.
func listIP(args []string, ifaces []net.Interface) ([]string, error) {
	all := []string{}
	var isIface bool
	for _, a := range args {
		isIface = false
		for _, iface := range ifaces {
			if a == iface.Name {
				isIface = true
				addrs, err := iface.Addrs()
				if err != nil {
					return nil, fmt.Errorf("failed to get the IP addresses of the interface: %q", a)
				}
				for _, addr := range addrs {
					if ipnet, ok := addr.(*net.IPNet); ok {
						if ipnet.IP.To4() != nil || (!ipnet.IP.IsLinkLocalMulticast() && !ipnet.IP.IsLinkLocalUnicast()) {
							all = append(all, ipnet.IP.String())
						}
					}
				}
			}
		}
		if !isIface {
			if net.ParseIP(a) != nil {
				all = append(all, a)
			} else {
				addrs, err := net.LookupHost(a)
				if err != nil {
					return nil, fmt.Errorf("not a valid IP address, interface or hostname: %q", a)
				}
				all = append(all, addrs...)
			}
		}
	}
	return all, nil
}
