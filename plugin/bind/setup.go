package bind

import (
	"errors"
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
	ifaces, err := net.Interfaces()
	if err != nil {
		return plugin.Error("bind", fmt.Errorf("failed to get interfaces list"))
	}

	for c.Next() {
		b, err := parse(c)
		if err != nil {
			return plugin.Error("bind", err)
		}

		includes, err := listIP(b.includes, ifaces)
		if err != nil {
			return err
		}

		excludes, err := listIP(b.excludes, ifaces)
		if err != nil {
			return err
		}

		for _, ip := range includes {
			if !isIn(ip, excludes) {
				all = append(all, ip)
			}
		}
	}

	config.ListenHosts = all
	return nil
}

func parse(c *caddy.Controller) (*bind, error) {
	b := &bind{}
	b.includes = c.RemainingArgs()
	if len(b.includes) == 0 {
		return nil, plugin.Error("bind", fmt.Errorf("at least one address or interface name is expected"))
	}
	for c.NextBlock() {
		switch c.Val() {
		case "exclude":
			b.excludes = c.RemainingArgs()
			if len(b.excludes) == 0 {
				return nil, errors.New("at least one exclude must be given to exclude subdirective")
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
					return nil, plugin.Error("bind", fmt.Errorf("failed to get the IP addresses of the interface: %q", a))
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
			if net.ParseIP(a) == nil {
				return nil, plugin.Error("bind", fmt.Errorf("not a valid IP address or interface name: %q", a))
			}
			all = append(all, a)
		}
	}
	return all, nil
}

// isIn checks if a string array contains an element
func isIn(s string, list []string) bool {
	is := false
	for _, l := range list {
		if s == l {
			is = true
		}
	}
	return is
}
