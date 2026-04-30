package proxy

import (
	"net"
)

type transportType int

const (
	typeUDP transportType = iota
	typeTCP
	typeTotalCount // keep this last
)

func stringToTransportType(s string) transportType {
	switch s {
	case "udp":
		return typeUDP
	case "tcp", "tcp-tls":
		return typeTCP
	default:
		return typeUDP
	}
}

func (t *Transport) transportTypeFromConn(pc *persistConn) transportType {
	if _, ok := pc.c.Conn.(*net.UDPConn); ok {
		return typeUDP
	}

	return typeTCP
}
