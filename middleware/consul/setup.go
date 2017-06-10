package consul

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/pkg/dnsutil"
	"github.com/coredns/coredns/middleware/pkg/singleflight"
	mwtls "github.com/coredns/coredns/middleware/pkg/tls"
	"github.com/coredns/coredns/middleware/proxy"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("consul", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	e, stubzones, err := consulParse(c)
	if err != nil {
		return middleware.Error("Consul", err)
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

func consulParse(c *caddy.Controller) (*Consul, bool, error) {
	stub := make(map[string]proxy.Proxy)
	consul := Consul{
		// Don't default to a proxy for lookups.
		//		Proxy:      proxy.NewLookup([]string{"8.8.8.8:53", "8.8.4.4:53"}),
		PathPrefix: "coredns",
		Inflight:   &singleflight.Group{},
		Stubmap:    &stub,
	}
	var (
		tlsConfig *tls.Config
		err       error
		endpoint  = defaultEndpoint
		stubzones = false
	)
	for c.Next() {
		if c.Val() == "consul" {
			consul.Zones = c.RemainingArgs()
			if len(consul.Zones) == 0 {
				consul.Zones = make([]string, len(c.ServerBlockKeys))
				copy(consul.Zones, c.ServerBlockKeys)
			}
			for i, str := range consul.Zones {
				consul.Zones[i] = middleware.Host(str).Normalize()
			}

			if c.NextBlock() {
				for {
					switch c.Val() {
					case "stubzones":
						stubzones = true
					case "debug":
						consul.Debugging = true
					case "path":
						if !c.NextArg() {
							return &Consul{}, false, c.ArgErr()
						}
						consul.PathPrefix = c.Val()
					case "endpoint":
						args := c.RemainingArgs()
						if len(args) != 1 {
							return &Consul{}, false, c.ArgErr()
						}
						endpoint = args[0]
					case "upstream":
						args := c.RemainingArgs()
						if len(args) == 0 {
							return &Consul{}, false, c.ArgErr()
						}
						ups, err := dnsutil.ParseHostPortOrFile(args...)
						if err != nil {
							return &Consul{}, false, err
						}
						consul.Proxy = proxy.NewLookup(ups)
					case "tls": // cert key cacertfile
						args := c.RemainingArgs()
						tlsConfig, err = mwtls.NewTLSConfigFromArgs(args...)
						if err != nil {
							return &Consul{}, false, err
						}
					default:
						if c.Val() != "}" {
							return &Consul{}, false, c.Errf("unknown property '%s'", c.Val())
						}
					}

					if !c.Next() {
						break
					}
				}
			}
			client, err := newConsulClient(endpoint, tlsConfig)
			if err != nil {
				return &Consul{}, false, err
			}
			consul.Client = client
			consul.ConsulKV = client.KV()
			consul.endpoint = endpoint

			return &consul, stubzones, nil
		}
	}
	return &Consul{}, false, nil
}

func newConsulClient(endpoint string, cc *tls.Config) (*consulapi.Client, error) {
	config := consulapi.DefaultConfig()
	config.Address = endpoint
	config.Transport = newHTTPSTransport(cc)

	// config.Scheme is handled in NewClient()
	client, err := consulapi.NewClient(config)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func newHTTPSTransport(cc *tls.Config) *http.Transport {
	// this seems like a bad idea but was here in the previous version
	if cc != nil {
		cc.InsecureSkipVerify = true
	}

	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     cc,
	}

	return tr
}

const defaultEndpoint = "http://localhost:8500"
