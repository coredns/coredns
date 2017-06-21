package etcdv2

import (
	"context"
	"crypto/tls"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/backend"
	"github.com/coredns/coredns/middleware/pkg/dnsutil"
	"github.com/coredns/coredns/middleware/pkg/singleflight"
	mwtls "github.com/coredns/coredns/middleware/pkg/tls"
	"github.com/coredns/coredns/middleware/proxy"

	etcdc "github.com/coreos/etcd/client"
	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("etcd", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	e, stubzones, err := EtcdParse(c)
	if err != nil {
		return middleware.Error("etcd", err)
	}

	if stubzones {
		c.OnStartup(func() error {
			e.UpdateStubZones()
			return nil
		})
	}

	dnsserver.GetConfig(c).AddMiddleware(func(next middleware.Handler) middleware.Handler {
		e.Next = next
		return e
	})

	return nil
}

func EtcdParse(c *caddy.Controller) (*backend.Backend, bool, error) {
	stub := make(map[string]proxy.Proxy)
	etc := EtcdV2{
		// Don't default to a proxy for lookups.
		//		Proxy:      proxy.NewLookup([]string{"8.8.8.8:53", "8.8.4.4:53"}),
		PathPrefix: "skydns",
		Ctx:        context.Background(),
		Inflight:   &singleflight.Group{},
	}
	b := backend.Backend{
		ServiceName:    "etcdv2",
		Stubmap:        &stub,
		ServiceBackend: &etc,
	}
	var (
		tlsConfig *tls.Config
		err       error
		endpoints = []string{defaultEndpoint}
		stubzones = false
	)
	for c.Next() {
		if c.Val() == "etcd" {
			b.Zones = c.RemainingArgs()
			if len(b.Zones) == 0 {
				b.Zones = make([]string, len(c.ServerBlockKeys))
				copy(b.Zones, c.ServerBlockKeys)
			}
			for i, str := range b.Zones {
				b.Zones[i] = middleware.Host(str).Normalize()
			}

			if c.NextBlock() {
				for {
					switch c.Val() {
					case "stubzones":
						stubzones = true
					case "debug":
						b.Debugging = true
					case "path":
						if !c.NextArg() {
							return nil, false, c.ArgErr()
						}
						etc.PathPrefix = c.Val()
					case "endpoint":
						args := c.RemainingArgs()
						if len(args) == 0 {
							return nil, false, c.ArgErr()
						}
						endpoints = args
					case "upstream":
						args := c.RemainingArgs()
						if len(args) == 0 {
							return nil, false, c.ArgErr()
						}
						ups, err := dnsutil.ParseHostPortOrFile(args...)
						if err != nil {
							return nil, false, err
						}
						etc.Proxy = proxy.NewLookup(ups)
					case "tls": // cert key cacertfile
						args := c.RemainingArgs()
						tlsConfig, err = mwtls.NewTLSConfigFromArgs(args...)
						if err != nil {
							return nil, false, err
						}
					default:
						if c.Val() != "}" {
							return nil, false, c.Errf("unknown property '%s'", c.Val())
						}
					}

					if !c.Next() {
						break
					}
				}
			}
			client, err := NewEtcdClient(endpoints, tlsConfig)
			if err != nil {
				return nil, false, err
			}
			etc.Client = client
			etc.Endpoints = endpoints

			return &b, stubzones, nil
		}
	}
	return &backend.Backend{}, false, nil
}

func NewEtcdClient(endpoints []string, cc *tls.Config) (etcdc.KeysAPI, error) {
	etcdCfg := etcdc.Config{
		Endpoints: endpoints,
		Transport: mwtls.NewHTTPSTransport(cc),
	}
	cli, err := etcdc.New(etcdCfg)
	if err != nil {
		return nil, err
	}
	return etcdc.NewKeysAPI(cli), nil
}

const defaultEndpoint = "http://localhost:2379"
