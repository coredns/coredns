// Package proxy is middleware that proxies requests.
package proxy

import (
	"crypto/tls"
	"errors"
	"sync/atomic"
	"time"

	"github.com/miekg/coredns/middleware"
	"github.com/miekg/coredns/middleware/grpc/pb"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	creds "google.golang.org/grpc/credentials"

)

var errUnreachable = errors.New("unreachable backend")
var errInvalidEncoding = errors.New("invalid encoding")

// Proxy represents a middleware instance that can proxy requests to another DNS server.
type Proxy struct {
	Next      middleware.Handler
	Client    *client
	Upstreams []Upstream
}

// Upstream manages a pool of proxy upstream hosts. Select should return a
// suitable upstream host, or nil if no such hosts are available.
type Upstream interface {
	// The domain name this upstream host should be routed on.
	From() string
	// Selects an upstream host to be routed to.
	Select() *UpstreamHost
	// Checks if subpdomain is not an ignored.
	IsAllowedPath(string) bool
	// Options returns the options set for this upstream
	Options() Options
}

// UpstreamHostDownFunc can be used to customize how Down behaves.
type UpstreamHostDownFunc func(*UpstreamHost) bool

// UpstreamHost represents a single proxy upstream
type UpstreamHost struct {
	Conns             int64  // must be first field to be 64-bit aligned on 32-bit systems
	Name              string // IP address (and port) of this upstream host
	Fails             int32
	FailTimeout       time.Duration
	Unhealthy         bool
	CheckDown         UpstreamHostDownFunc
	WithoutPathPrefix string
	encoding	  upstreamEncoding
	tls		  *tls.Config
	grpc		  pb.DnsServiceClient
}

// Down checks whether the upstream host is down or not.
// Down will try to use uh.CheckDown first, and will fall
// back to some default criteria if necessary.
func (uh *UpstreamHost) Down() bool {
	if uh.CheckDown == nil {
		// Default settings
		fails := atomic.LoadInt32(&uh.Fails)
		return uh.Unhealthy || fails > 0
	}
	return uh.CheckDown(uh)
}

func (uh *UpstreamHost) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg, p Proxy) (*dns.Msg, error) {

	var reply *dns.Msg
	var backendErr error

	atomic.AddInt64(&uh.Conns, 1)

	switch uh.encoding {
		case encodingNone:
			reply, backendErr = p.Client.ServeDNS(w, r, uh)
		case encodingGRPC:
			reply, backendErr = uh.serveGRPC(ctx, w, r)
		case encodingGRPCTLS:
			reply, backendErr = uh.serveGRPC(ctx, w, r)
		default:
			reply, backendErr =  nil, errInvalidEncoding
	}
	atomic.AddInt64(&uh.Conns, -1)

	return reply, backendErr
}

func (uh *UpstreamHost) serveGRPC(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (*dns.Msg, error) {
	if uh.grpc == nil {
		var conn *grpc.ClientConn
		var err error
		if uh.tls != nil {
			conn, err = grpc.Dial(uh.Name, grpc.WithTransportCredentials(creds.NewTLS(uh.tls)))
		} else {
			conn, err = grpc.Dial(uh.Name, grpc.WithInsecure())
		}
		if err != nil {
			return nil, err
		}
		uh.grpc = pb.NewDnsServiceClient(conn)
	}

	msg, err := r.Pack()
	if err != nil {
		return nil, err
	}

	reply, err := uh.grpc.Query(ctx, &pb.DnsPacket{Msg: msg})
	if err != nil {
		return nil, err
	}
	d := new(dns.Msg)
	err = d.Unpack(reply.Msg)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// tryDuration is how long to try upstream hosts; failures result in
// immediate retries until this duration ends or we get a nil host.
var tryDuration = 60 * time.Second

// ServeDNS satisfies the middleware.Handler interface.
func (p Proxy) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	for _, upstream := range p.Upstreams {
		start := time.Now()

		// Since Select() should give us "up" hosts, keep retrying
		// hosts until timeout (or until we get a nil host).
		for time.Now().Sub(start) < tryDuration {
			host := upstream.Select()
			if host == nil {

				RequestDuration.WithLabelValues(upstream.From()).Observe(float64(time.Since(start) / time.Millisecond))

				return dns.RcodeServerFailure, errUnreachable
			}

			reply, backendErr := host.ServeDNS(ctx, w, r, p)

			if backendErr == nil {
				w.WriteMsg(reply)

				RequestDuration.WithLabelValues(upstream.From()).Observe(float64(time.Since(start) / time.Millisecond))

				return 0, nil
			}
			timeout := host.FailTimeout
			if timeout == 0 {
				timeout = 10 * time.Second
			}
			atomic.AddInt32(&host.Fails, 1)
			go func(host *UpstreamHost, timeout time.Duration) {
				time.Sleep(timeout)
				atomic.AddInt32(&host.Fails, -1)
			}(host, timeout)
		}

		RequestDuration.WithLabelValues(upstream.From()).Observe(float64(time.Since(start) / time.Millisecond))

		return dns.RcodeServerFailure, errUnreachable
	}
	return middleware.NextOrFailure(p.Name(), p.Next, ctx, w, r)
}

// Name implements the Handler interface.
func (p Proxy) Name() string { return "proxy" }

// defaultTimeout is the default networking timeout for DNS requests.
const defaultTimeout = 5 * time.Second
