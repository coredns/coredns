package ipscope

import "net"

func InterfaceAddrs() (IPSet, error) {
	var ipset IPSet
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		var ip net.IP
		switch a := addr.(type) {
		case *net.IPNet:
			ip = a.IP
		case *net.IPAddr:
			ip = a.IP
		}
		ipset = append(ipset, ip)
	}
	return ipset, nil
}
