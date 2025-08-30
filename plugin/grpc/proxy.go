package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/coredns/coredns/pb"

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

	// connection
	client   pb.DnsServiceClient
	dialOpts []grpc.DialOption
}

// newProxy returns a new proxy.
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

	reply, err := p.client.Query(ctx, &pb.DnsPacket{Msg: msg})
	if err != nil {
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
