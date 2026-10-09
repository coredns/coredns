//go:build linux

package proxy

import (
	"syscall"

	"github.com/coredns/coredns/plugin/pkg/log"

	"golang.org/x/sys/unix"
)

// recvErr is a net.Dialer Control function that sets IP_RECVERR or IPV6_RECVERR
// on UDP sockets. Without it Linux passes only some ICMP errors, such as port
// unreachable, to a connected UDP socket, and drops net and host unreachable
// (net/ipv4/udp.c, __udp4_lib_err). A read from an upstream behind an
// unreachable route then waits out its timeout. With it, the read fails as soon
// as the ICMP error arrives. Other networks are left alone.
func recvErr(network, _ string, c syscall.RawConn) error {
	if network != "udp4" && network != "udp6" {
		return nil
	}
	return c.Control(func(fd uintptr) {
		const maxInt = int(^uint(0) >> 1)
		if fd > uintptr(maxInt) {
			log.Warningf("Failed to set IP_RECVERR on socket: invalid file descriptor %d", fd)
			return
		}
		level, opt := unix.SOL_IP, unix.IP_RECVERR
		if network == "udp6" {
			level, opt = unix.SOL_IPV6, unix.IPV6_RECVERR
		}
		if err := unix.SetsockoptInt(int(fd), level, opt, 1); err != nil { // #nosec G115 -- fd is range-checked above
			log.Warningf("Failed to set IP_RECVERR on socket: %s", err)
		}
	})
}
