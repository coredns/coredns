package ipscope

import "net"

const (
	scopeUnknown = iota
	scopeClassA
	scopeClassB
	scopeClassC
	scopeUniqueLocal
	scopeLinkLocal
	scopeLoopback
	scopeGlobal
	scopeLast
)

type IPSet []net.IP

type IPScopes [scopeLast]IPSet

func NewIPScopes(ipset IPSet) *IPScopes {
	var scopes IPScopes
	for _, ip := range ipset {
		scopes.Put(ip)
	}
	return &scopes
}

func (scopes *IPScopes) Put(ip net.IP) {
	scope := getScopeByIP(ip)
	scopes[scope] = append(scopes[scope], ip)
}

func (scopes IPScopes) Get(ip net.IP) IPSet {
	scope := getScopeByIP(ip)
	return scopes[scope]
}

func (ipset IPSet) To4() IPSet {
	var ipset4 IPSet
	for _, ip := range ipset {
		if ip.To4() != nil {
			ipset4 = append(ipset4, ip)
		}
	}
	return ipset4
}

func (ipset IPSet) To6() IPSet {
	var ipset6 IPSet
	for _, ip := range ipset {
		if ip.To4() == nil && ip.To16() != nil {
			ipset6 = append(ipset6, ip)
		}
	}
	return ipset6
}

func getScopeByIP(ip net.IP) uint {
	if classA.Contains(ip) {
		return scopeClassA
	}
	if classB.Contains(ip) {
		return scopeClassB
	}
	if classC.Contains(ip) {
		return scopeClassC
	}
	if uniqueLocal.Contains(ip) {
		return scopeUniqueLocal
	}
	if ip.IsLinkLocalUnicast() {
		return scopeLinkLocal
	}
	if ip.IsLoopback() {
		return scopeLoopback
	}
	if ip.IsGlobalUnicast() {
		return scopeGlobal
	}
	return scopeUnknown
}
