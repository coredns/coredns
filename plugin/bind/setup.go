package bind

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"syscall"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/log"
)

var (
	netInterfaces     = net.Interfaces
	getInterfaceAddrs = defaultInterfaceAddrs
	checkBind         = defaultCheckBind
)

func defaultInterfaceAddrs(iface net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}

func defaultCheckBind(addr string) error {
	ln, err := net.Listen("tcp", net.JoinHostPort(addr, "0"))
	if err != nil {
		return err
	}
	_ = ln.Close()

	pc, err := net.ListenPacket("udp", net.JoinHostPort(addr, "0"))
	if err != nil {
		return err
	}
	_ = pc.Close()

	return nil
}

func isAddrNotAvailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EADDRNOTAVAIL) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cannot assign requested address") ||
		strings.Contains(msg, "address not available") ||
		strings.Contains(msg, "not valid in its context")
}

func setup(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)
	// addresses will be consolidated over all BIND directives available in that BlocServer
	all := []string{}
	ifaces, err := netInterfaces()
	if err != nil {
		log.Warning(plugin.Error("bind", fmt.Errorf("failed to get interfaces list, cannot bind by interface name: %s", err)))
	}

	for c.Next() {
		b, err := parse(c)
		if err != nil {
			return plugin.Error("bind", err)
		}

		ips, err := listIP(b.addrs, ifaces, true)
		if err != nil {
			return plugin.Error("bind", err)
		}

		except, err := listIP(b.except, ifaces, false)
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
		return nil, errors.New("at least one address or interface name is expected")
	}
	for c.NextBlock() {
		switch c.Val() {
		case "except":
			b.except = c.RemainingArgs()
			if len(b.except) == 0 {
				return nil, errors.New("at least one address or interface must be given to except subdirective")
			}
		default:
			return nil, fmt.Errorf("invalid option %q", c.Val())
		}
	}
	return b, nil
}

// listIP returns a list of IP addresses from a list of arguments which can be either IP-Address or Interface-Name.
// When checkAssign is true, interface addresses are verified to be assignable.
func listIP(args []string, ifaces []net.Interface, checkAssign bool) ([]string, error) {
	all := []string{}
	var isIface bool
	for _, a := range args {
		isIface = false
		for _, iface := range ifaces {
			if a == iface.Name {
				isIface = true
				addrs, err := getInterfaceAddrs(iface)
				if err != nil {
					return nil, fmt.Errorf("failed to get the IP addresses of the interface: %q", a)
				}

				var ifaceIPs []string
				for _, addr := range addrs {
					if ipnet, ok := addr.(*net.IPNet); ok {
						ipa, err := net.ResolveIPAddr("ip", ipnet.IP.String())
						if err == nil {
							if ipnet.IP.To4() == nil &&
								(ipnet.IP.IsLinkLocalMulticast() || ipnet.IP.IsLinkLocalUnicast()) {
								if ipa.Zone == "" {
									ipa.Zone = iface.Name
								}
							}
							ifaceIPs = append(ifaceIPs, ipa.String())
						}
					}
				}

				if !checkAssign {
					all = append(all, ifaceIPs...)
					continue
				}

				var validAddrs []string
				type nonAssignable struct {
					addr string
					err  error
				}
				var skipped []nonAssignable

				for _, ipStr := range ifaceIPs {
					if err := checkBind(ipStr); err != nil {
						if isAddrNotAvailable(err) {
							skipped = append(skipped, nonAssignable{addr: ipStr, err: err})
							continue
						}
						return nil, fmt.Errorf("failed to bind interface address %s: %w", ipStr, err)
					}
					validAddrs = append(validAddrs, ipStr)
				}

				if len(validAddrs) > 0 {
					for _, s := range skipped {
						log.Warning(plugin.Error("bind", fmt.Errorf("cannot assign requested address %s on interface %s: %w", s.addr, iface.Name, s.err)))
					}
					all = append(all, validAddrs...)
				} else if len(skipped) > 0 {
					return nil, fmt.Errorf("failed to bind to any address on interface %q: %w", iface.Name, skipped[0].err)
				}
			}
		}
		if !isIface {
			if net.ParseIP(a) == nil {
				return nil, fmt.Errorf("not a valid IP address or interface name: %q", a)
			}
			all = append(all, a)
		}
	}
	return all, nil
}
