package msg

import (
	"net"
	"testing"

	tap "github.com/dnstap/golang-dnstap"
)

// ipv4MappedINET is the 16-byte IPv4-mapped IPv6 form of 192.0.2.1
// (::ffff:192.0.2.1). Dual-stack listeners report IPv4 clients this way, and
// Go's net.IP.To4 still recognizes them as IPv4.
var ipv4MappedINET = net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 192, 0, 2, 1}

// An IPv4 client reported as an IPv4-mapped IPv6 address must be stored as a
// 4-octet address with SocketFamily INET, otherwise the dnstap message parser
// could get confused when expecting only 4 bytes but is presented with 16 bytes.
func TestSetAddressIPv4MappedReportedAsINET(t *testing.T) {
	assertINET := func(t *testing.T, family tap.SocketFamily, addr []byte) {
		t.Helper()
		if family != tap.SocketFamily_INET {
			t.Errorf("SocketFamily = %v, want INET", family)
		}
		if len(addr) != 4 {
			t.Errorf("address length = %d, want 4 octets for INET", len(addr))
		}
		if got := net.IP(addr).String(); got != "192.0.2.1" {
			t.Errorf("address = %q, want %q", got, "192.0.2.1")
		}
	}

	t.Run("query/udp", func(t *testing.T) {
		m := new(tap.Message)
		if err := SetQueryAddress(m, &net.UDPAddr{IP: ipv4MappedINET}); err != nil {
			t.Fatal(err)
		}
		assertINET(t, m.GetSocketFamily(), m.GetQueryAddress())
	})

	t.Run("query/tcp", func(t *testing.T) {
		m := new(tap.Message)
		if err := SetQueryAddress(m, &net.TCPAddr{IP: ipv4MappedINET}); err != nil {
			t.Fatal(err)
		}
		assertINET(t, m.GetSocketFamily(), m.GetQueryAddress())
	})

	t.Run("response/udp", func(t *testing.T) {
		m := new(tap.Message)
		if err := SetResponseAddress(m, &net.UDPAddr{IP: ipv4MappedINET}); err != nil {
			t.Fatal(err)
		}
		assertINET(t, m.GetSocketFamily(), m.GetResponseAddress())
	})

	t.Run("response/tcp", func(t *testing.T) {
		m := new(tap.Message)
		if err := SetResponseAddress(m, &net.TCPAddr{IP: ipv4MappedINET}); err != nil {
			t.Fatal(err)
		}
		assertINET(t, m.GetSocketFamily(), m.GetResponseAddress())
	})
}

// Native IPv4 addresses keep SocketFamily INET and a 4-octet address.
func TestSetAddressNativeIPv4(t *testing.T) {
	m := new(tap.Message)
	if err := SetQueryAddress(m, &net.UDPAddr{IP: net.IP{192, 0, 2, 1}}); err != nil {
		t.Fatal(err)
	}
	if m.GetSocketFamily() != tap.SocketFamily_INET {
		t.Errorf("SocketFamily = %v, want INET", m.GetSocketFamily())
	}
	if got := len(m.GetQueryAddress()); got != 4 {
		t.Errorf("address length = %d, want 4 octets for INET", got)
	}
}

// Genuine IPv6 addresses keep SocketFamily INET6 and a 16-octet address.
func TestSetAddressNativeIPv6(t *testing.T) {
	m := new(tap.Message)
	if err := SetQueryAddress(m, &net.UDPAddr{IP: net.ParseIP("2001:db8::1")}); err != nil {
		t.Fatal(err)
	}
	if m.GetSocketFamily() != tap.SocketFamily_INET6 {
		t.Errorf("SocketFamily = %v, want INET6", m.GetSocketFamily())
	}
	if got := len(m.GetQueryAddress()); got != 16 {
		t.Errorf("address length = %d, want 16 octets for INET6", got)
	}
}
