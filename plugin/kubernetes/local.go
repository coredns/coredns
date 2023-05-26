package kubernetes

import (
	"net"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

// boundIPs returns the list of non-loopback IPs that CoreDNS is bound to
func boundIPs(c *caddy.Controller) (ips []net.IP) {
	conf := dnsserver.GetConfig(c)
	hosts := conf.ListenHosts
	if hosts == nil || hosts[0] == "" {
		hosts = nil
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			return nil
		}
		for _, addr := range addrs {
			hosts = append(hosts, addr.String())
		}
	}
	for _, host := range hosts {
		ip, _, _ := net.ParseCIDR(host)
		ip4 := ip.To4()
		if ip4 != nil && !ip4.IsLoopback() {
			ips = append(ips, ip4)
			continue
		}
		ip6 := ip.To16()
		if ip6 != nil && !ip6.IsLoopback() {
			ips = append(ips, ip6)
		}
	}
	return ips
}
