package msg

import (
	"fmt"
	"net"
	"time"

	tap "github.com/dnstap/golang-dnstap"
)

var (
	protoUDP    = tap.SocketProtocol_UDP
	protoTCP    = tap.SocketProtocol_TCP
	familyINET  = tap.SocketFamily_INET
	familyINET6 = tap.SocketFamily_INET6
)

// socketFamilyAndAddress maps an IP to the dnstap SocketFamily and the address
// bytes to store for it. IPv4, including IPv4-mapped IPv6, is returned as a
// 4-octet INET address; anything else as a 16-octet INET6 address, matching
// https://github.com/dnstap/dnstap.pb/blob/main/dnstap.proto.
func socketFamilyAndAddress(ip net.IP) (*tap.SocketFamily, []byte) {
	if ip4 := ip.To4(); ip4 != nil {
		return &familyINET, ip4
	}
	return &familyINET6, ip
}

// TODO: SetQueryAddress and SetResponseAddress each set the message-level SocketFamily (and SocketProtocol) from their own address. A dnstap Message describes a single socket whose two endpoints share one family, but calling both setters with addresses of different families leaves SocketFamily reflecting only the last call and inconsistent with the other stored address. Evaluate replacing the two setters with a single SetAddresses(t, query, response) that derives SocketFamily/SocketProtocol once for the whole socket and rejects a family mismatch.

// SetQueryAddress adds the query address to the message. This also sets the SocketFamily and SocketProtocol.
func SetQueryAddress(t *tap.Message, addr net.Addr) error {
	switch a := addr.(type) {
	case *net.TCPAddr:
		t.SocketProtocol = &protoTCP

		p := uint32(a.Port) // #nosec G115 -- Port is inherently bounded (1-65535)
		t.QueryPort = &p

		t.SocketFamily, t.QueryAddress = socketFamilyAndAddress(a.IP)
		return nil
	case *net.UDPAddr:
		t.SocketProtocol = &protoUDP

		p := uint32(a.Port) // #nosec G115 -- Port is inherently bounded (1-65535)
		t.QueryPort = &p

		t.SocketFamily, t.QueryAddress = socketFamilyAndAddress(a.IP)
		return nil
	default:
		return fmt.Errorf("unknown address type: %T", a)
	}
}

// SetResponseAddress the response address to the message. This also sets the SocketFamily and SocketProtocol.
func SetResponseAddress(t *tap.Message, addr net.Addr) error {
	switch a := addr.(type) {
	case *net.TCPAddr:
		t.SocketProtocol = &protoTCP

		p := uint32(a.Port) // #nosec G115 -- Port is inherently bounded (1-65535)
		t.ResponsePort = &p

		t.SocketFamily, t.ResponseAddress = socketFamilyAndAddress(a.IP)
		return nil
	case *net.UDPAddr:
		t.SocketProtocol = &protoUDP

		p := uint32(a.Port) // #nosec G115 -- Port is inherently bounded (1-65535)
		t.ResponsePort = &p

		t.SocketFamily, t.ResponseAddress = socketFamilyAndAddress(a.IP)
		return nil
	default:
		return fmt.Errorf("unknown address type: %T", a)
	}
}

// SetQueryTime sets the time of the query in t.
func SetQueryTime(t *tap.Message, ti time.Time) {
	qts := uint64(ti.Unix())       // #nosec G115 -- Unix time fits in uint64
	qtn := uint32(ti.Nanosecond()) // #nosec G115 -- Nanoseconds (0-999999999) fit in uint32
	t.QueryTimeSec = &qts
	t.QueryTimeNsec = &qtn
}

// SetResponseTime sets the time of the response in t.
func SetResponseTime(t *tap.Message, ti time.Time) {
	rts := uint64(ti.Unix())       // #nosec G115 -- Unix time fits in uint64
	rtn := uint32(ti.Nanosecond()) // #nosec G115 -- Nanoseconds (0-999999999) fit in uint32
	t.ResponseTimeSec = &rts
	t.ResponseTimeNsec = &rtn
}

// SetType sets the type in t.
func SetType(t *tap.Message, typ tap.Message_Type) { t.Type = &typ }
