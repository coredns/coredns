package kubernetes

import (
	"net"
)

func localPodIP() (ips []net.IP) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}

	for _, addr := range addrs {
		ip, _, _ := net.ParseCIDR(addr.String())
		ip4 := ip.To4()
		if ip4 != nil && !ip4.IsLoopback() {
			ips = append(ips, ip4)
		}
		ip6 := ip.To16()
		if ip6 != nil && !ip6.IsLoopback() {
			ips = append(ips, ip6)
		}
	}
	return ips
}

// LocalNodeName is exclusively used in federation plugin, will be deprecated later.
func (k *Kubernetes) LocalNodeName() string {
	localIPs := k.interfaceAddrsFunc()
	if len(localIPs) == 0 {
		return ""
	}

	// Find fist endpoint matching any localIP
	for _, localIP := range localIPs {
		for _, ep := range k.APIConn.EpIndexReverse(localIP.String()) {
			for _, eps := range ep.Subsets {
				for _, addr := range eps.Addresses {
					if localIP.Equal(net.ParseIP(addr.IP)) {
						return addr.NodeName
					}
				}
			}
		}
	}
	return ""
}
