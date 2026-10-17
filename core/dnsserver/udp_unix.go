//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package dnsserver

import (
	"net"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"golang.org/x/sys/unix"
)

var udpOOBSize = len(ipv4.NewControlMessage(ipv4.FlagDst|ipv4.FlagInterface)) +
	len(ipv6.NewControlMessage(ipv6.FlagDst|ipv6.FlagInterface))

func enableUDPPacketInfo(conn *net.UDPConn) bool {
	err4 := ipv4.NewPacketConn(conn).SetControlMessage(ipv4.FlagDst|ipv4.FlagInterface, true)
	err6 := ipv6.NewPacketConn(conn).SetControlMessage(ipv6.FlagDst|ipv6.FlagInterface, true)
	return err4 == nil || err6 == nil
}

func parseUDPPacketInfo(oob []byte, flags int) udpPacketInfo {
	if flags&unix.MSG_CTRUNC != 0 {
		return udpPacketInfo{}
	}
	cm6 := new(ipv6.ControlMessage)
	if cm6.Parse(oob) == nil && cm6.Dst != nil {
		return udpPacketInfo{dst: cm6.Dst, ifIndex: cm6.IfIndex}
	}
	cm4 := new(ipv4.ControlMessage)
	if cm4.Parse(oob) == nil && cm4.Dst != nil {
		return udpPacketInfo{dst: cm4.Dst, ifIndex: cm4.IfIndex}
	}
	return udpPacketInfo{}
}

func udpSourceControl(info udpPacketInfo) []byte {
	if info.dst == nil || info.dst.IsUnspecified() || info.dst.IsMulticast() {
		return nil
	}
	if ip4 := info.dst.To4(); ip4 != nil {
		return (&ipv4.ControlMessage{Src: ip4, IfIndex: info.replyInterface()}).Marshal()
	}
	return (&ipv6.ControlMessage{Src: info.dst, IfIndex: info.replyInterface()}).Marshal()
}
