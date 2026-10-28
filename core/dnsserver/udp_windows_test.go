package dnsserver

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"net"
	"testing"

	"golang.org/x/sys/windows"
)

func TestUDPPacketInfoWindows(t *testing.T) {
	// Native WSACMSGHDR followed by IN_PKTINFO/IN6_PKTINFO. These fixtures
	// include the receiving interface, unlike unscoped send control messages.
	ipv4 := "180000000000000000000000130000007f00000107000000"
	ipv6 := "24000000000000002900000013000000fe8000000000000000000000000000010700000000000000"
	if udpCmsgWord == 4 {
		ipv4 = "1400000000000000130000007f00000107000000"
		ipv6 = "200000002900000013000000fe80000000000000000000000000000107000000"
	}
	for _, tc := range []struct{ name, encoded, dst string }{
		{"ipv4", ipv4, "127.0.0.1"},
		{"ipv6", ipv6, "fe80::1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, err := hex.DecodeString(tc.encoded)
			if err != nil {
				t.Fatal(err)
			}
			info := parseUDPPacketInfo(control, 0)
			if !info.dst.Equal(net.ParseIP(tc.dst)) || info.ifIndex != 7 {
				t.Fatalf("packet info = %+v, want %s on interface 7", info, tc.dst)
			}
			if got := parseUDPPacketInfo(control, windows.MSG_CTRUNC); got.dst != nil {
				t.Fatal("truncated control data must not supply a destination")
			}
			for n := range udpCmsgHeader + len(info.dst) + 4 {
				if got := parseUDPPacketInfo(control[:n], 0); got.dst != nil {
					t.Errorf("accepted truncated control data of length %d", n)
				}
			}
			if tc.name == "ipv4" {
				// Global/loopback replies retain route selection; only scoped
				// IPv6 replies carry an outgoing interface constraint.
				binary.NativeEndian.PutUint32(control[udpCmsgHeader+4:], 0)
			}
			if got := udpSourceControl(info); !bytes.Equal(got, control) {
				t.Errorf("source control = %x, want %x", got, control)
			}
		})
	}
}

func TestUDPPacketInfoWindowsMalformed(t *testing.T) {
	valid := udpSourceControl(udpPacketInfo{dst: net.ParseIP("127.0.0.1")})
	for _, length := range []uint64{0, udpCmsgHeader - 1, udpCmsgHeader, udpCmsgHeader + 3, uint64(len(valid) + 1), ^uint64(0)} {
		control := bytes.Clone(valid)
		if udpCmsgWord == 8 {
			binary.NativeEndian.PutUint64(control, length)
		} else {
			binary.NativeEndian.PutUint32(control, uint32(length))
		}
		if got := parseUDPPacketInfo(control, 0); got.dst != nil {
			t.Errorf("accepted control length %d", length)
		}
	}
	unknown := bytes.Clone(valid)
	binary.NativeEndian.PutUint32(unknown[udpCmsgWord+4:], 0xffff)
	control := append(unknown, valid...)
	if got := parseUDPPacketInfo(control, 0); !got.dst.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("did not find packet info after an unknown message: %+v", got)
	}
}
