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

// SetQueryAddress adds the query address to the message. This also sets the SocketFamily and SocketProtocol.
func SetQueryAddress(t *tap.Message, addr net.Addr) error {
	t.SocketFamily = &familyINET
	switch a := addr.(type) {
	case *net.TCPAddr:
		t.SocketProtocol = &protoTCP
		t.QueryAddress = a.IP

		p := uint32(a.Port)
		t.QueryPort = &p

		if a.IP.To4() == nil {
			t.SocketFamily = &familyINET6
		}
		return nil
	case *net.UDPAddr:
		t.SocketProtocol = &protoUDP
		t.QueryAddress = a.IP

		p := uint32(a.Port)
		t.QueryPort = &p

		if a.IP.To4() == nil {
			t.SocketFamily = &familyINET6
		}
		return nil
	default:
		return fmt.Errorf("unknown address type: %T", a)
	}
}

// SetResponseAddress the response address to the message. This also sets the SocketFamily and SocketProtocol.
func SetResponseAddress(t *tap.Message, addr net.Addr) error {
	t.SocketFamily = &familyINET
	switch a := addr.(type) {
	case *net.TCPAddr:
		t.SocketProtocol = &protoTCP
		t.ResponseAddress = a.IP

		p := uint32(a.Port)
		t.ResponsePort = &p

		if a.IP.To4() == nil {
			t.SocketFamily = &familyINET6
		}
		return nil
	case *net.UDPAddr:
		t.SocketProtocol = &protoUDP
		t.ResponseAddress = a.IP

		p := uint32(a.Port)
		t.ResponsePort = &p

		if a.IP.To4() == nil {
			t.SocketFamily = &familyINET6
		}
		return nil
	default:
		return fmt.Errorf("unknown address type: %T", a)
	}
}

// SetQueryTime sets the time of the query in t.
func SetQueryTime(t *tap.Message, ti time.Time) {
	qts := uint64(ti.Unix())
	qtn := uint32(ti.Nanosecond())
	t.QueryTimeSec = &qts
	t.QueryTimeNsec = &qtn
}

// SetResponseTime sets the time of the response in t.
func SetResponseTime(t *tap.Message, ti time.Time) {
	rts := uint64(ti.Unix())
	rtn := uint32(ti.Nanosecond())
	t.ResponseTimeSec = &rts
	t.ResponseTimeNsec = &rtn
}

// SetType sets the type in t.
func SetType(t *tap.Message, typ tap.Message_Type) { t.Type = &typ }
