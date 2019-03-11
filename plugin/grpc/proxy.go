package grpc

import (
	"context"
	"crypto/tls"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/pb"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/up"
	"github.com/miekg/dns"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// Proxy defines an upstream host.
type Proxy struct {
	fails uint32
	addr  string

	// connection
	client   pb.DnsServiceClient
	dialOpts []grpc.DialOption

	// health checking
	probe *up.Probe
}

// GrpcOpts defines an GrpcProxy options.
type options struct {
	TLSConfig *tls.Config
}

// newProxy returns a new proxy.
func newProxy(addr string, opts *options) *Proxy {
	p := &Proxy{
		fails: 0,
		addr:  addr,
		probe: up.New(),
	}

	if opts.TLSConfig != nil {
		p.dialOpts = append(p.dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(opts.TLSConfig)))
	} else {
		p.dialOpts = append(p.dialOpts, grpc.WithInsecure())
	}

	conn, err := grpc.Dial(p.addr, p.dialOpts...)
	if err != nil {
		log.Warningf("Skipping gRPC host '%s' due to Dial error: %s\n", p.addr, err)
	} else {
		p.client = pb.NewDnsServiceClient(conn)
	}

	return p
}

// down returns true if this proxy is down, i.e. has *more* fails than maxfails.
func (p *Proxy) down(maxfails uint32) bool {
	if maxfails == 0 {
		return false
	}

	fails := atomic.LoadUint32(&p.fails)
	return fails > maxfails
}

// Stop stops the health checking goroutine.
func (p *Proxy) stop() { p.probe.Stop() }

// Start starts the proxy's healthchecking.
func (p *Proxy) start(duration time.Duration) {
	p.probe.Start(duration)
}

// query sends the request and waits for a response.
func (p *Proxy) query(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	start := time.Now()

	msg, err := req.Pack()
	if err != nil {
		return nil, err
	}

	reply, err := p.client.Query(ctx, &pb.DnsPacket{Msg: msg})
	if err != nil {
		// if not found error, return empty message with NXDomain code
		if status.Code(err) == codes.NotFound {
			m := new(dns.Msg).SetRcode(req, dns.RcodeNameError)
			return m, nil
		}
		return nil, err
	}
	ret := new(dns.Msg)
	if err := ret.Unpack(reply.Msg); err != nil {
		return nil, err
	}

	rc, ok := dns.RcodeToString[ret.Rcode]
	if !ok {
		rc = strconv.Itoa(ret.Rcode)
	}

	RequestCount.WithLabelValues(p.addr).Add(1)
	RcodeCount.WithLabelValues(rc, p.addr).Add(1)
	RequestDuration.WithLabelValues(p.addr).Observe(time.Since(start).Seconds())

	return ret, nil
}

// healthcheck kicks of a round of health checks for this proxy.
func (p *Proxy) healthcheck() {
	p.probe.Do(p.check)
}

// check is used as the up.Func in the up.Probe.
func (p *Proxy) check() error {
	msg := new(dns.Msg)
	msg.SetQuestion(".", dns.TypeNS)
	reqMsg, err := msg.Pack()
	if err != nil {
		return err
	}
	req := &pb.DnsPacket{Msg: reqMsg}

	// Pass a context with a timeout to tell a blocking function that it
	// should abandon its work after the timeout elapses.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err = p.client.Query(ctx, req)
	if err != nil {
		if status.Code(err) == codes.Unavailable {
			HealthcheckFailureCount.WithLabelValues(p.addr).Add(1)
			atomic.AddUint32(&p.fails, 1)
			return err
		}
	}

	atomic.StoreUint32(&p.fails, 0)
	return nil
}
