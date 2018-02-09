package dnsserver

import (
	"fmt"
	"net"
	"strings"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

type zoneAddr struct {
	Zone      string
	Port      string
	Transport string     // dns, tls or grpc
	IPNet     *net.IPNet // if reverse zone this hold the IPNet
}

// String return the string representation of z.
func (z zoneAddr) String() string { return z.Transport + "://" + z.Zone + ":" + z.Port }

// Transport returns the protocol of the string s
func Transport(s string) string {
	switch {
	case strings.HasPrefix(s, TransportTLS+"://"):
		return TransportTLS
	case strings.HasPrefix(s, TransportDNS+"://"):
		return TransportDNS
	case strings.HasPrefix(s, TransportGRPC+"://"):
		return TransportGRPC
	}
	return TransportDNS
}

// normalizeZone parses an zone string into a structured format with separate
// host, and port portions, as well as the original input string.
func normalizeZone(str string) (zoneAddr, error) {
	var err error

	// Default to DNS if there isn't a transport protocol prefix.
	trans := TransportDNS

	switch {
	case strings.HasPrefix(str, TransportTLS+"://"):
		trans = TransportTLS
		str = str[len(TransportTLS+"://"):]
	case strings.HasPrefix(str, TransportDNS+"://"):
		trans = TransportDNS
		str = str[len(TransportDNS+"://"):]
	case strings.HasPrefix(str, TransportGRPC+"://"):
		trans = TransportGRPC
		str = str[len(TransportGRPC+"://"):]
	}

	host, port, ipnet, err := plugin.SplitHostPort(str)
	if err != nil {
		return zoneAddr{}, err
	}

	if port == "" {
		if trans == TransportDNS {
			port = Port
		}
		if trans == TransportTLS {
			port = TLSPort
		}
		if trans == TransportGRPC {
			port = GRPCPort
		}
	}

	return zoneAddr{Zone: dns.Fqdn(host), Port: port, Transport: trans, IPNet: ipnet}, nil
}

// SplitProtocolHostPort - split a full formed address like "dns://[::1}:53" into parts
func SplitProtocolHostPort(address string) (protocol string, ip string, port string, err error) {
	parts := strings.Split(address, "://")
	switch len(parts) {
	case 1:
		ip, port, err := net.SplitHostPort(parts[0])
		return "", ip, port, err
	case 2:
		ip, port, err := net.SplitHostPort(parts[1])
		return parts[0], ip, port, err
	default:
		return "", "", "", fmt.Errorf("provided value is not in an address format : %s", address)
	}
}

// Supported transports.
const (
	TransportDNS  = "dns"
	TransportTLS  = "tls"
	TransportGRPC = "grpc"
)

const keyPartsSeparator = "##"

type addrKey struct {
	Transport string // dns, tls or grpc
	Zone      string
	Address   string
	Port      string
}

func (k *addrKey) asKey() string {
	return k.Transport + keyPartsSeparator + k.Zone + keyPartsSeparator + k.Address + keyPartsSeparator + k.Port
}

func parseAddrKey(key string) (*addrKey, error) {
	keyParts := strings.Split(key, keyPartsSeparator)
	if len(keyParts) != 4 {
		return nil, fmt.Errorf("Provided value is not a key for addrKey, expect 4 parts joined by '%s' : %s", key, keyPartsSeparator)
	}
	return &addrKey{keyParts[0], keyParts[1], keyParts[2], keyParts[3]}, nil
}

func (k *addrKey) isMulticast() bool {
	return k.Address == ""
}

func (k *addrKey) copyAsMulticast() *addrKey {
	return &addrKey{Zone: k.Zone, Address: "", Port: k.Port, Transport: k.Transport}

}

// String return the string representation of this ZoneAddr - for print
func (k *addrKey) String() string {
	if k.Address == "" {
		return k.Transport + "://" + k.Zone + ":" + k.Port
	}
	return k.Transport + "://" + k.Zone + ":" + k.Address + ":" + k.Port
}

// Build a Validator that rise error if the bound addresses for listeners are overlapping
type zoneAddrOverlapValidator struct {
	registeredAddr   map[string]string
	multicastOverlap map[string]string
}

func newZoneAddrOverlapValidator() *zoneAddrOverlapValidator {
	return &zoneAddrOverlapValidator{registeredAddr: make(map[string]string), multicastOverlap: make(map[string]string)}
}

func (c *zoneAddrOverlapValidator) registerAndCheck(k *addrKey) (same bool, overlap bool, overlapKey string) {
	key := k.asKey()
	if _, ok := c.registeredAddr[key]; ok {
		// exact same zone already registered
		return true, false, ""
	}
	mkey := k.copyAsMulticast().asKey()
	if already, ok := c.multicastOverlap[mkey]; ok {
		if k.isMulticast() {
			// there is already a unicast registered
			return false, true, already
		}
		if _, ok := c.registeredAddr[mkey]; ok {
			// the overlapping multicast is already registered
			return false, true, mkey
		}
	} else {
		c.multicastOverlap[mkey] = key
	}
	c.registeredAddr[key] = key
	return false, false, ""
}
