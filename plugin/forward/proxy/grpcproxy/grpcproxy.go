package grpcproxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/pb"
	"github.com/coredns/coredns/plugin/forward/metrics"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/up"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// GrpcProxy defines an upstream host.
type GrpcProxy struct {
	fails uint32

	addr string

	// Connection
	dialOpts []grpc.DialOption
	client   pb.DnsServiceClient

	// health checking
	probe *up.Probe

	// Metrics
	m *metrics.Metrics
}

// GrpcOpts defines an GrpcProxy options.
type GrpcOpts struct {
	TLSConfig *tls.Config
}

// New returns a new proxy.
func New(addr string, opts *GrpcOpts, metrics *metrics.Metrics) *GrpcProxy {
	p := &GrpcProxy{
		fails: 0,
		addr:  addr,
		probe: up.New(),
		m:     metrics,
	}

	if opts.TLSConfig != nil {
		p.dialOpts = append(p.dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(opts.TLSConfig)))
	} else {
		p.dialOpts = append(p.dialOpts, grpc.WithInsecure())
	}

	return p
}

// Query selects an upstream, sends the request and waits for a response.
func (p *GrpcProxy) Query(ctx context.Context, state request.Request) (*dns.Msg, error) {
	fmt.Printf("--- Query via gRPC %s --- \n", p.addr)
	start := time.Now()
	msg, err := state.Req.Pack()
	if err != nil {
		return nil, err
	}

	reply, err := p.client.Query(ctx, &pb.DnsPacket{Msg: msg})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			m := new(dns.Msg).SetRcode(state.Req, dns.RcodeNameError)
			return m, nil
		}
		return nil, err
	}
	ret := new(dns.Msg)
	err = ret.Unpack(reply.Msg)
	if err != nil {
		return nil, err
	}

	rc, ok := dns.RcodeToString[ret.Rcode]
	if !ok {
		rc = strconv.Itoa(ret.Rcode)
	}

	p.m.RequestCount.WithLabelValues(p.addr).Add(1)
	p.m.RcodeCount.WithLabelValues(rc, p.addr).Add(1)
	p.m.RequestDuration.WithLabelValues(p.addr).Observe(time.Since(start).Seconds())

	return ret, nil
}

// Healthcheck kicks of a round of health checks for this proxy.
func (p *GrpcProxy) Healthcheck() {
	p.probe.Do(func() error {
		return p.check()
	})
}

// Down returns true if this proxy is down, i.e. has *more* fails than maxfails.
func (p *GrpcProxy) Down(maxfails uint32) bool {
	if maxfails == 0 {
		return false
	}

	fails := atomic.LoadUint32(&p.fails)
	return fails > maxfails
}

// Stop stops the health checking goroutine.
func (p *GrpcProxy) Stop() { p.probe.Stop() }

// Start starts the proxy's healthchecking.
func (p *GrpcProxy) Start(duration time.Duration) {
	p.probe.Start(duration)

	conn, err := grpc.Dial(p.addr, p.dialOpts...)
	if err != nil {
		log.Warningf("Skipping gRPC host '%s' due to Dial error: %s\n", p.addr, err)
	} else {
		p.client = pb.NewDnsServiceClient(conn)
	}
}

// Address returns the proxy's address.
func (p *GrpcProxy) Address() string { return p.addr }

// FailsNum returns the proxy's fails number.
func (p *GrpcProxy) FailsNum() *uint32 { return &p.fails }

// check is used as the up.Func in the up.Probe.
func (p *GrpcProxy) check() error {
	fmt.Printf("--- Checking gRPC server %s --- \n", p.addr)
	msg := new(dns.Msg)
	msg.SetQuestion(".", dns.TypeNS)
	reqMsg, err := msg.Pack()
	if err != nil {
		return err
	}
	req := &pb.DnsPacket{Msg: reqMsg}

	ctx := context.Background()
	_, err = p.client.Query(ctx, req)
	if err != nil {
		if status.Code(err) == codes.Unavailable {
			p.m.HealthcheckFailureCount.WithLabelValues(p.addr).Add(1)
			atomic.AddUint32(&p.fails, 1)
			return err
		}
	}

	atomic.StoreUint32(&p.fails, 0)
	return nil
}
