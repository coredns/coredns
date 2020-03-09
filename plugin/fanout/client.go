package fanout

import (
	"crypto/tls"
	"fmt"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"time"
)

// Client represents the proxy for remote DNS server
type Client interface {
	Request(request.Request) (*dns.Msg, error)
	Endpoint() string
	SetTLSConfig(*tls.Config)
}

type client struct {
	transport Transport
	addr      string
	net       string
}

// NewClient creates new client with specific addr and network
func NewClient(addr, net string) Client {
	a := &client{
		addr:      addr,
		net:       net,
		transport: NewTransport(addr),
	}
	return a
}

// SetTLSConfig sets tls config for client
func (c *client) SetTLSConfig(cfg *tls.Config) {
	if cfg != nil {
		c.net = tcptlc
	}
	c.transport.SetTLSConfig(cfg)
}

// Endpoint returns address of DNS server
func (c *client) Endpoint() string {
	return c.addr
}

// Request sends request to DNS server
func (c *client) Request(request request.Request) (*dns.Msg, error) {
	start := time.Now()
	conn, err := c.transport.Dial(c.net)
	if err != nil {
		return nil, err
	}
	defer func() {
		logErrIfNotNil(conn.Close())
	}()

	logErrIfNotNil(conn.SetWriteDeadline(time.Now().Add(maxTimeout)))
	if err = conn.WriteMsg(request.Req); err != nil {
		logErrIfNotNil(err)
		return nil, err
	}
	logErrIfNotNil(conn.SetReadDeadline(time.Now().Add(readTimeout)))
	var ret *dns.Msg
	for {
		ret, err = conn.ReadMsg()
		if err != nil {
			logErrIfNotNil(err)
			return nil, err
		}
		if request.Req.Id == ret.Id {
			break
		}
	}
	rc, ok := dns.RcodeToString[ret.Rcode]
	if !ok {
		rc = fmt.Sprint(ret.Rcode)
	}
	RequestCount.WithLabelValues(c.addr).Add(1)
	RcodeCount.WithLabelValues(rc, c.addr).Add(1)
	RequestDuration.WithLabelValues(c.addr).Observe(time.Since(start).Seconds())
	return ret, nil
}
