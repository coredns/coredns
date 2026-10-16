//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package dnsserver

import "net"

const udpOOBSize = 0

func enableUDPPacketInfo(*net.UDPConn) bool        { return false }
func parseUDPPacketInfo([]byte, int) udpPacketInfo { return udpPacketInfo{} }
func udpSourceControl(udpPacketInfo) []byte        { return nil }
