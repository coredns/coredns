package dnsserver

import (
	"net"
	"strconv"
	"strings"

	"fmt"
	"github.com/coredns/coredns/plugin"
	"github.com/miekg/dns"
	"sort"
)

//ZoneAddr is the memory structure representation of Keys for ServerBlocs
// use String() to get a print representation
// use asKey() if you need to use this ZoneAddr in a map
// use ParseZoneAddr() if you need to retrieve the initial ZoneAddr from the key format
type ZoneAddr struct {
	// first 4 are part of the key
	Transport     string // dns, tls or grpc
	Zone          string
	Port          string
	ListeningAddr string
	IPNet         *net.IPNet // if reverse zone this hold the IPNet
	Options       map[string]string
}

const (
	separateProtocol  = "://"
	separatePortAndIP = ":"
	separateCIDR      = "##"
	separateOptions   = "#$#"
	startOption       = "["
	endOption         = "]"
	partsSep          = "="
)

var transports map[int]string

// Copy : provide a duplicate of this ZoneAddr
func (z *ZoneAddr) Copy() ZoneAddr {
	za := ZoneAddr{Transport: z.Transport, Zone: z.Zone, Port: z.Port, ListeningAddr: z.ListeningAddr, IPNet: z.IPNet}
	for k, v := range z.Options {
		za.Options[k] = v
	}
	return za
}

func (z ZoneAddr) copyAsMulticast() ZoneAddr {
	return ZoneAddr{Zone: z.Zone, ListeningAddr: "", Port: z.Port, Transport: z.Transport, IPNet: z.IPNet}

}

// CompleteAddress : update ListeningAddr if not already set
func (z *ZoneAddr) CompleteAddress(address string) {
	if (z.ListeningAddr == "") && (address != "") {
		z.ListeningAddr = address
	}
}

// String return the string representation of this ZoneAddr - for print
func (z ZoneAddr) String() string {
	if z.ListeningAddr == "" {
		return z.Transport + separateProtocol + z.Zone + separatePortAndIP + z.Port
	}
	return z.Transport + separateProtocol + z.Zone + separatePortAndIP + z.ListeningAddr + separatePortAndIP + z.Port
}

// return a string representation that is usable for  of this ZoneAddr - for print
func (z ZoneAddr) keyForListener() string {
	if z.ListeningAddr == "" {
		return z.Transport + separateProtocol + z.Zone + separatePortAndIP + z.Port
	}
	return z.Transport + separateProtocol + z.Zone + separatePortAndIP + z.ListeningAddr + separatePortAndIP + z.Port
}

func (z ZoneAddr) isMulticast() bool {
	return z.ListeningAddr == ""
}

func (z ZoneAddr) serverAddr() string {
	if len(z.ListeningAddr) > 0 && z.ListeningAddr[0] == '[' {
		return z.ListeningAddr[1 : len(z.ListeningAddr)-1]
	}
	return z.ListeningAddr
}

// normalizeZone parses an zone string into a structured format with separate
// host, and port portions, as well as the original input string.
func normalizeZone(str string) (*ZoneAddr, error) {
	var err error

	// Default to DNS if there isn't a transport protocol prefix.
	trans := TransportDNS

	switch {
	case strings.HasPrefix(str, TransportTLS+separateProtocol):
		trans = TransportTLS
		str = str[len(TransportTLS+separateProtocol):]
	case strings.HasPrefix(str, TransportDNS+separateProtocol):
		trans = TransportDNS
		str = str[len(TransportDNS+separateProtocol):]
	case strings.HasPrefix(str, TransportGRPC+separateProtocol):
		trans = TransportGRPC
		str = str[len(TransportGRPC+separateProtocol):]
	}

	host, ip, port, ipnet, err := plugin.SplitHostPort(str)
	if err != nil {
		return nil, err
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

	// at the end we should verify that the host is a real dns domain
	if _, ok := dns.IsDomainName(host); !ok {
		return nil, fmt.Errorf("invalid format for zone, it is not considered as a dns domain : '%v'", host)
	}

	return &ZoneAddr{Zone: dns.Fqdn(host), ListeningAddr: ip, Port: port, Transport: trans, IPNet: ipnet}, nil
}

// Transport returns the protocol of the string s
func Transport(s string) string {
	switch {
	case strings.HasPrefix(s, TransportTLS+separateProtocol):
		return TransportTLS
	case strings.HasPrefix(s, TransportDNS+separateProtocol):
		return TransportDNS
	case strings.HasPrefix(s, TransportGRPC+separateProtocol):
		return TransportGRPC
	}
	return TransportDNS
}

// Supported transports.
const (
	TransportDNS  = "dns"
	TransportTLS  = "tls"
	TransportGRPC = "grpc"
)

//parseFromKey build a ZoneAddr from standard format that includes all options
func parseFromKey(value string) (*ZoneAddr, error) {
	// value is a an output of the ZoneAddr produced by asKey()
	// including all parts : protocol, domain, optional IP, port, option set of options

	v := strings.Split(value, separateOptions)
	// left part is limited to rotocol, domain, optional IP, port

	// right parts are the options
	if len(v[0]) == 0 {
		return nil, fmt.Errorf("invalid format for a ZoneAddress, it should not contains ony options : '%v'", value)
	}

	// extract the protocol
	head := strings.Split(v[0], "://")
	if len(head) != 2 {
		return nil, fmt.Errorf("invalid format for a ZoneAddress, it should start with a listening protocol : '%v'", head)
	}
	proto := head[0]
	tail := strings.Split(head[1], separateCIDR)
	corp := tail[0]
	zone, ip, port := "", "", ""
	// extract Zone, maybe IP, and port
	zoneAddr := plugin.SplitWithComment(corp, ':', '[', ']')
	switch len(zoneAddr) {
	case 2:
		zone, ip, port = zoneAddr[0], "", zoneAddr[1]
	case 3:
		zone, ip, port = zoneAddr[0], zoneAddr[1], zoneAddr[2]
	default:
		return nil, fmt.Errorf("invalid format for a ZoneAddress, it should contains host, IP and port : '%v'", corp)
	}
	if zone == "" {
		return nil, fmt.Errorf("invalid format for zone, it should not be empwty : '%v'", corp)
	}
	if _, ok := dns.IsDomainName(zone); !ok {
		return nil, fmt.Errorf("invalid format for zone, it is not considered as a dns domain : '%v'", zone)
	}
	if ip != "" {
		if ok, _ := plugin.IsIP(ip); !ok {
			return nil, fmt.Errorf("invalid format for ip, it is not an Ipv4 or Ipv6 format : '%v'", ip)
		}
	}
	_, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("invalid format for port, it is not an integer : '%v', error is %v ", value, err)
	}

	options := make(map[string]string)
	for _, ov := range v[:1] {
		if strings.Index(ov, startOption) == 0 && strings.LastIndex(ov, endOption) == len(ov)-len(endOption) {
			opt := ov[len(startOption) : len(ov)-len(endOption)]
			parts := strings.Split(opt, partsSep)
			if len(parts[0]) == 0 {
				return nil, fmt.Errorf("invalid option with no name : %v", opt)
			}
			if len(parts) > 2 {
				return nil, fmt.Errorf("invalid option with too much values : %v", opt)
			}
			options[parts[0]] = parts[1]
		}
	}
	za := ZoneAddr{proto, zone, port, ip, nil, options}
	if len(tail) > 1 {
		sipnet := tail[1]
		_, ipnet, err := net.ParseCIDR(sipnet)
		if err != nil {
			return nil, fmt.Errorf("invalid format for a ZoneAddress, cannot parse the network CIDR provided : '%v' - %v", sipnet, err)
		}
		za.IPNet = ipnet
	}

	return &za, nil
}

func (z ZoneAddr) asKey() string {
	head := z.String()
	if z.IPNet != nil {
		head = head + separateCIDR + z.IPNet.String()
	}
	if len(z.Options) > 0 {
		// Need to order the map to get the same key on each print
		oNames := make([]string, len(z.Options))
		i := 0
		for k := range z.Options {
			oNames[i] = k
			i++
		}
		sort.Strings(oNames)
		for _, k := range oNames {
			head += separateOptions + startOption + k + partsSep + z.Options[k] + endOption
		}
	}
	return head
}

// Build a Validator that rise error if the bound addresses for listeners are overlapping
type zoneAddrOverlapValidator struct {
	registeredAddr   map[string]string
	multicastOverlap map[string]string
}

func newZoneAddrOverlapValidator() *zoneAddrOverlapValidator {
	return &zoneAddrOverlapValidator{registeredAddr: make(map[string]string), multicastOverlap: make(map[string]string)}
}

func (c *zoneAddrOverlapValidator) registerAndCheck(z ZoneAddr) (same bool, overlap bool, overlapKey string) {
	key := z.asKey()
	if _, ok := c.registeredAddr[key]; ok {
		// exact same zone already registered
		return true, false, ""
	}
	mkey := z.copyAsMulticast().asKey()
	if already, ok := c.multicastOverlap[mkey]; ok {
		if z.isMulticast() {
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
