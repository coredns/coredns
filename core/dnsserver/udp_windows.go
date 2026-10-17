package dnsserver

import (
	"encoding/binary"
	"math"
	"net"
	"strconv"

	"golang.org/x/sys/windows"
)

// WSACMSGHDR contains a native-size length followed by two 32-bit integers.
// Each control message is aligned to SIZE_T; its length excludes tail padding.
const udpCmsgWord = strconv.IntSize / 8
const udpCmsgHeader = udpCmsgWord + 8

var udpOOBSize = udpCmsgAlign(udpCmsgHeader+8) + udpCmsgAlign(udpCmsgHeader+20)

func udpCmsgAlign(n int) int { return (n + udpCmsgWord - 1) & ^(udpCmsgWord - 1) }

func enableUDPPacketInfo(conn *net.UDPConn) bool {
	raw, err := conn.SyscallConn()
	if err != nil {
		return false
	}
	var err4, err6 error
	err = raw.Control(func(fd uintptr) {
		// Dual-stack sockets need both options to receive both families.
		err4 = windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IP, windows.IP_PKTINFO, 1)
		err6 = windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IPV6, windows.IPV6_PKTINFO, 1)
	})
	return err == nil && (err4 == nil || err6 == nil)
}

func parseUDPPacketInfo(oob []byte, flags int) udpPacketInfo {
	if flags&windows.MSG_CTRUNC != 0 {
		return udpPacketInfo{}
	}
	for len(oob) >= udpCmsgHeader {
		var length uint64
		if udpCmsgWord == 8 {
			length = binary.NativeEndian.Uint64(oob)
		} else {
			length = uint64(binary.NativeEndian.Uint32(oob))
		}
		if length < udpCmsgHeader || length > uint64(len(oob)) || length > math.MaxInt32 {
			return udpPacketInfo{}
		}
		level := binary.NativeEndian.Uint32(oob[udpCmsgWord:])
		kind := binary.NativeEndian.Uint32(oob[udpCmsgWord+4:])
		data := oob[udpCmsgHeader:length]
		ipLen := 0
		switch {
		case level == windows.IPPROTO_IP && kind == windows.IP_PKTINFO:
			ipLen = net.IPv4len
		case level == windows.IPPROTO_IPV6 && kind == windows.IPV6_PKTINFO:
			ipLen = net.IPv6len
		}
		if ipLen != 0 {
			if len(data) < ipLen+4 {
				return udpPacketInfo{}
			}
			ifIndex := binary.NativeEndian.Uint32(data[ipLen:])
			if ifIndex > math.MaxInt32 {
				return udpPacketInfo{}
			}
			return udpPacketInfo{dst: net.IP(data[:ipLen]), ifIndex: int(ifIndex)}
		}
		next := udpCmsgAlign(int(length))
		if next < int(length) || next > len(oob) {
			break
		}
		oob = oob[next:]
	}
	return udpPacketInfo{}
}

func udpSourceControl(info udpPacketInfo) []byte {
	if info.dst == nil || info.dst.IsUnspecified() || info.dst.IsMulticast() {
		return nil
	}
	ifIndex := info.replyInterface()
	if ifIndex < 0 || ifIndex > math.MaxInt32 {
		return nil
	}
	ip := info.dst.To4()
	level, kind := uint32(windows.IPPROTO_IP), uint32(windows.IP_PKTINFO)
	if ip == nil {
		ip = info.dst.To16()
		level, kind = windows.IPPROTO_IPV6, windows.IPV6_PKTINFO
	}
	if ip == nil {
		return nil
	}
	length := udpCmsgHeader + len(ip) + 4
	oob := make([]byte, udpCmsgAlign(length))
	if udpCmsgWord == 8 {
		binary.NativeEndian.PutUint64(oob, uint64(length))
	} else {
		binary.NativeEndian.PutUint32(oob, uint32(length)) //nolint:gosec // Header plus a 4- or 16-byte IP and a 4-byte interface.
	}
	binary.NativeEndian.PutUint32(oob[udpCmsgWord:], level)
	binary.NativeEndian.PutUint32(oob[udpCmsgWord+4:], kind)
	copy(oob[udpCmsgHeader:], ip)
	binary.NativeEndian.PutUint32(oob[udpCmsgHeader+len(ip):], uint32(ifIndex))
	return oob
}
