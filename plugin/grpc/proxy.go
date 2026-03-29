package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/pb"
	"github.com/coredns/coredns/plugin/pkg/up"

	"github.com/miekg/dns"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	// maxDNSMessageBytes is the maximum size of a DNS message on the wire.
	maxDNSMessageBytes = dns.MaxMsgSize

	// maxProtobufPayloadBytes accounts for protobuf overhead.
	// Field tag=1 (1 byte) + length varint for 65535 (3 bytes) = 4 bytes total
	maxProtobufPayloadBytes = maxDNSMessageBytes + 4
)

var (
	// ErrDNSMessageTooLarge is returned when a DNS message exceeds the maximum allowed size.
	ErrDNSMessageTooLarge = errors.New("dns message exceeds size limit")
)

// Proxy defines an upstream host.
type Proxy struct {
	addr string

	// single-connection mode (default, pool_size=1): zero overhead, unchanged from upstream
	client   pb.DnsServiceClient
	dialOpts []grpc.DialOption

	// pooled mode (pool_size > 1, opt-in only)
	transport  *grpcTransport
	probe      *up.Probe
	hcInterval time.Duration
	fails      uint32 // atomic counter
}

// proxyOptions holds configuration for creating a pooled proxy.
type proxyOptions struct {
	poolSize   int
	maxFails   uint32
	hcInterval time.Duration
}

// newProxy returns a new proxy with a single shared connection.
// This is the default (pool_size=1) path, identical to the upstream model.
func newProxy(addr string, tlsConfig *tls.Config) (*Proxy, error) {
	p := &Proxy{
		addr: addr,
	}

	if tlsConfig != nil {
		p.dialOpts = append(p.dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		p.dialOpts = append(p.dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Cap send/recv sizes to avoid oversized messages.
	// Note: gRPC size limits apply to the serialized protobuf message size.
	p.dialOpts = append(p.dialOpts,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxProtobufPayloadBytes),
			grpc.MaxCallSendMsgSize(maxProtobufPayloadBytes),
		),
	)

	conn, err := grpc.NewClient(p.addr, p.dialOpts...)
	if err != nil {
		return nil, err
	}
	p.client = pb.NewDnsServiceClient(conn)

	return p, nil
}

// newPooledProxy returns a new proxy with connection pooling and optional health checking.
// Only used when pool_size > 1 is explicitly configured.
func newPooledProxy(addr string, tlsConfig *tls.Config, opts *proxyOptions) (*Proxy, error) {
	p := &Proxy{
		addr: addr,
	}

	var dialOpts []grpc.DialOption
	if tlsConfig != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	dialOpts = append(dialOpts,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxProtobufPayloadBytes),
			grpc.MaxCallSendMsgSize(maxProtobufPayloadBytes),
		),
	)

	p.transport = newTransport("grpc", p.addr, dialOpts, opts.poolSize)

	// Only create probe and set hcInterval when health checking is opted into.
	if opts.maxFails > 0 {
		p.probe = up.New()
		p.hcInterval = opts.hcInterval
	}

	return p, nil
}

// query sends the request and waits for a response.
func (p *Proxy) query(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	start := time.Now()

	msg, err := req.Pack()
	if err != nil {
		return nil, err
	}

	if err := validateDNSSize(msg); err != nil {
		return nil, err
	}

	// Resolve which client to use: pooled or single-connection.
	var (
		client pb.DnsServiceClient
		gc     *grpcConn
	)
	if p.transport != nil {
		gc, err = p.transport.Dial(ctx)
		if err != nil {
			return nil, err
		}
		client = gc.client
	} else {
		client = p.client
	}

	reply, err := client.Query(ctx, &pb.DnsPacket{Msg: msg})
	if err != nil {
		if gc != nil && status.Code(err) == codes.Unavailable {
			p.transport.RemoveConn(gc)
		}
		// if not found message, return empty message with NXDomain code
		if status.Code(err) == codes.NotFound {
			m := new(dns.Msg).SetRcode(req, dns.RcodeNameError)
			return m, nil
		}
		return nil, err
	}

	wire := reply.GetMsg()
	if err := validateDNSSize(wire); err != nil {
		return nil, err
	}

	ret := new(dns.Msg)
	if err := ret.Unpack(wire); err != nil {
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

func validateDNSSize(data []byte) error {
	l := len(data)
	if l > maxDNSMessageBytes {
		return fmt.Errorf("%w: %d bytes (limit %d)", ErrDNSMessageTooLarge, l, maxDNSMessageBytes)
	}
	return nil
}

// Down returns true if this proxy has exceeded the max failure threshold.
// Only meaningful for pooled proxies with health checking; always returns false when maxfails is 0.
func (p *Proxy) Down(maxfails uint32) bool {
	if maxfails == 0 {
		return false
	}
	fails := atomic.LoadUint32(&p.fails)
	return fails > maxfails
}

// Healthcheck triggers a health check probe for this proxy.
// Only active for pooled proxies with health checking enabled (probe is nil otherwise).
func (p *Proxy) Healthcheck() {
	if p.probe == nil {
		return
	}
	p.probe.Do(func() error {
		return p.healthCheck()
	})
}

// healthCheck performs a health check by sending a DNS query.
func (p *Proxy) healthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	gc, err := p.transport.Dial(ctx)
	if err != nil {
		atomic.AddUint32(&p.fails, 1)
		HealthcheckFailureCount.WithLabelValues(p.addr).Add(1)
		return err
	}

	ping := new(dns.Msg)
	ping.SetQuestion(".", dns.TypeNS)
	msg, err := ping.Pack()
	if err != nil {
		atomic.AddUint32(&p.fails, 1)
		HealthcheckFailureCount.WithLabelValues(p.addr).Add(1)
		return err
	}

	_, err = gc.client.Query(ctx, &pb.DnsPacket{Msg: msg})
	if err != nil {
		atomic.AddUint32(&p.fails, 1)
		HealthcheckFailureCount.WithLabelValues(p.addr).Add(1)
		if status.Code(err) == codes.Unavailable {
			p.transport.RemoveConn(gc)
		}
		return err
	}

	atomic.StoreUint32(&p.fails, 0)
	return nil
}

// Start starts the proxy's transport and health checking.
// This is a no-op for single-connection proxies.
func (p *Proxy) Start() {
	if p.transport != nil {
		p.transport.Start()
	}
	if p.probe != nil {
		p.probe.Start(p.hcInterval)
	}
}

// Stop stops the proxy's transport and health checking.
// This is a no-op for single-connection proxies.
func (p *Proxy) Stop() {
	if p.probe != nil {
		p.probe.Stop()
	}
	if p.transport != nil {
		p.transport.Stop()
	}
}
