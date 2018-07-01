package variables

import (
	"github.com/coredns/coredns/request"
)

const (
	// These return a string.
	qName      = "qname"
	clientIP   = "client_ip"
	clientPort = "client_port"
	serverIP   = "server_ip"
	serverPort = "server_port"
	// These return an int.
	qType = "qtype"
	proto = "protocol"
)

var (
	// String is a list of available variables that return a string.
	String = []string{qName, qType, clientIP, clientPort, serverIP, serverPort}
	// Int is a list of available variables that return a int.
	Int = []string{qType, proto}
)

// StringValue return string value for variable v. It return the empty string if nothing could be returned.
func StringValue(state request.Request, v string) string {
	switch v {
	case qName:
		return state.QName()
	case clientIP:
		return state.IP()
	case clientPort:
		return state.Port()
	case serverIP:
		return state.LocalAddr()
	case serverPort:
		return state.LocalPort()
	case proto:
		return state.Proto()
	}

	return ""
}

// IntValue return the int value for variable v, -1 is returns if nothing could be found.
func IntValue(state request.Request, v string) int {
	switch v {
	case qType:
		return int(state.QType())

	}
	return -1
}
