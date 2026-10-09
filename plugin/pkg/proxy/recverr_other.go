//go:build !linux

package proxy

import "syscall"

// recvErr does nothing outside Linux.
func recvErr(_, _ string, _ syscall.RawConn) error { return nil }
