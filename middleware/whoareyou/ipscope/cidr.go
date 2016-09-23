package ipscope

import "net"

func init() {
	classA = mustParseCIDRIPNet("10.0.0.0/8")
	classB = mustParseCIDRIPNet("172.16.0.0/12")
	classC = mustParseCIDRIPNet("192.168.0.0/16")
	uniqueLocal = mustParseCIDRIPNet("fc00::/7")
}

var (
	classA, classB, classC, uniqueLocal *net.IPNet
)

func mustParseCIDRIPNet(s string) *net.IPNet {
	_, ret, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return ret
}
