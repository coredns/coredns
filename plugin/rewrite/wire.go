package rewrite

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// parseIP parses an address as returned by request.Request's IP and LocalIP.
// For an IPv6 link-local address these include the zone, e.g. "fe80::1%eth0",
// which net.ParseIP does not accept, so the zone is removed first.
func parseIP(addr string) net.IP {
	if i := strings.IndexByte(addr, '%'); i >= 0 {
		addr = addr[:i]
	}
	return net.ParseIP(addr)
}

// ipToWire writes IP address to wire/binary format, 4 or 16 bytes depends on IPV4 or IPV6.
func ipToWire(family int, ipAddr string) ([]byte, error) {
	switch family {
	case 1:
		return parseIP(ipAddr).To4(), nil
	case 2:
		return parseIP(ipAddr).To16(), nil
	}
	return nil, fmt.Errorf("invalid IP address family (i.e. version) %d", family)
}

// uint16ToWire writes unit16 to wire/binary format
func uint16ToWire(data uint16) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, data)
	return buf
}

// portToWire writes port to wire/binary format, 2 bytes
func portToWire(portStr string) ([]byte, error) {
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}
	return uint16ToWire(uint16(port)), nil
}
