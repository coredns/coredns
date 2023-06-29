package dnsserver

import (
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"net"
	"net/http"
)

// DoQWriter is a nonwriter.Writer that adds more specific LocalAddr and RemoteAddr methods.
type DoQWriter struct {
	nonwriter.Writer
	// raddr is the remote's address. This can be optionally set.
	raddr net.Addr
	// laddr is our address. This can be optionally set.
	laddr net.Addr

	// request is the HTTP request we're currently handling.
	request *http.Request
}

// RemoteAddr returns the remote address.
func (d *DoQWriter) RemoteAddr() net.Addr { return d.raddr }

// LocalAddr returns the local address.
func (d *DoQWriter) LocalAddr() net.Addr { return d.laddr }
