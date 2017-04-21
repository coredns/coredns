// Package dns64 implements a middleware that performs DNS64.
package dns64

import (
	"log"
	"net"

	"github.com/mholt/caddy/caddyhttp/proxy"
	"github.com/miekg/coredns/middleware"
	"github.com/miekg/coredns/middleware/pkg/response"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

// DNS64 performs DNS64.
type DNS64 struct {
	Next  middleware.Handler
	Proxy proxy.Proxy
}

// ServeDNS implements the middleware.Handler interface.
func (d DNS64) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	drr := &ResponseWriter{w}
	return d.Next.ServeDNS(ctx, drr, r)
}

// Name implements the Handler interface.
func (d DNS64) Name() string { return "dns64" }

// ResponseWriter is a response writer that implements DNS64, when an AAAA query returns
// NODATA, it will try and fetch any A records and synthesize the AAAA records on the fly.
type ResponseWriter struct {
	dns.ResponseWriter
}

// WriteMsg implements the dns.ResponseWriter interface.
func (r *ResponseWriter) WriteMsg(res *dns.Msg) error {
	// Only respond with this when the request came in over IPv6.
	v4 := false
	if ip, ok := r.RemoteAddr().(*net.UDPAddr); ok {
		v4 = ip.IP.To4() != nil
	}
	if ip, ok := r.RemoteAddr().(*net.TCPAddr); ok {
		v4 = ip.IP.To4() != nil
	}
	if v4 { // if it came in over v4, don't do anything.
		return r.ResponseWriter.WriteMsg(res)
	}

	ty, _ := response.Typify(res)
	// PTR records.
	if ty != response.NoData {
		return r.ResponseWriter.WriteMsg(res)
	}

	return r.ResponseWriter.WriteMsg(res)
}

// Write implements the dns.ResponseWriter interface.
func (r *ResponseWriter) Write(buf []byte) (int, error) {
	log.Printf("[WARNING] Dns64 called with Write: not performing DNS64")
	n, err := r.ResponseWriter.Write(buf)
	return n, err
}

// Hijack implements the dns.ResponseWriter interface.
func (r *ResponseWriter) Hijack() {
	r.ResponseWriter.Hijack()
	return
}

const pref64 = "64:ff9b::" // /9
