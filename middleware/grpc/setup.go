package grpc

import (
	"errors"

	"github.com/miekg/coredns/core/dnsserver"
	"github.com/miekg/coredns/middleware"
	"github.com/miekg/coredns/middleware/pkg/tls"

	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("grpc", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	g, err := grpcParse(c)
	if err != nil {
		return middleware.Error("grpc", err)
	}

	c.OnStartup(func() error {
		go g.Startup()
		return nil
	})

	c.OnShutdown(func() error {
		return g.Shutdown()
	})

	return nil
}

func grpcParse(c *caddy.Controller) (*grpc, error) {
	config := dnsserver.GetConfig(c)

	g := &grpc{config: config}

	for c.Next() {
		addr := c.RemainingArgs()

		if len(addr) > 0 {
			g.addr = addr[0]
		}

		for c.NextBlock() {
			switch c.Val() {
			case "tls": // cert key cacertfile
				args := c.RemainingArgs()
				if len(args) != 3 {
					return nil, c.ArgErr()
				}
				tls, err := tls.NewTLSConfig(args[0], args[1], args[2])
				if err != nil {
					return nil, c.ArgErr()
				}
				g.tls = tls
			}
		}
		return g, nil
	}
	return nil, errors.New("grpc setup called without keyword 'grpc' in Corefile")
}
