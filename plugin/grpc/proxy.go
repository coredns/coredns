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
	"google.golang.org/grpc/keepalive"
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

	// connection pooling
	dialOpts  []grpc.DialOption
	transport *grpcTransport

	// health checking
	probe    *up.Probe
	fails    uint32 // atomic counter
	maxFails uint32
}

// proxyOptions holds configuration for creating a proxy.
type proxyOptions struct {
	poolSize int
	expire   time.Duration
	maxFails uint32
}

// newProxy returns a new proxy with connection pooling and health checking.
func newProxy(addr string, tlsConfig *tls.Config, opts *proxyOptions) (*Proxy, error) {
	p := &Proxy{
		addr:     addr,
		maxFails: opts.maxFails,
		probe:    up.New(),
	}

	if tlsConfig != nil {
		p.dialOpts = append(p.dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		p.dialOpts = append(p.dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Add keepalive parameters for connection health
	p.dialOpts = append(p.dialOpts,
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                opts.expire,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)

	p.dialOpts = append(p.dialOpts,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxProtobufPayloadBytes),
			grpc.MaxCallSendMsgSize(maxProtobufPayloadBytes),
		),
	)

	p.transport = newTransport("grpc", p.addr, p.dialOpts, opts.poolSize, opts.expire)

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

	gc, _, err := p.transport.Dial(ctx)
	if err != nil {
		return nil, err
	}

	reply, err := gc.client.Query(ctx, &pb.DnsPacket{Msg: msg})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			m := new(dns.Msg).SetRcode(req, dns.RcodeNameError)
			return m, nil
		}
		if status.Code(err) == codes.Unavailable {
			p.transport.RemoveConn(gc)
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
func (p *Proxy) Down(maxfails uint32) bool {
	if maxfails == 0 {
		return false
	}
	fails := atomic.LoadUint32(&p.fails)
	return fails >= maxfails
}

// Healthcheck triggers a health check probe for this proxy.
// Uses the single-flight pattern to prevent concurrent health checks.
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

	gc, _, err := p.transport.Dial(ctx)
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
func (p *Proxy) Start(hcInterval time.Duration) {
	if p.transport != nil {
		p.transport.Start()
	}
	if p.probe != nil {
		p.probe.Start(hcInterval)
	}
}

// Stop stops the proxy's transport and health checking.
func (p *Proxy) Stop() {
	if p.probe != nil {
		p.probe.Stop()
	}
	if p.transport != nil {
		p.transport.Stop()
	}
}
