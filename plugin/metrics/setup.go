package metrics

import (
	"crypto/tls"
	"fmt"
	"net"
	"runtime"

	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/coremain"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics/vars"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/uniq"
)

var (
	log      = clog.NewWithPlugin("prometheus")
	u        = uniq.New()
	registry = newReg()
)

func init() { plugin.Register("prometheus", setup) }

func setup(c *caddy.Controller) error {
	m, err := parse(c)
	if err != nil {
		return plugin.Error("prometheus", err)
	}
	m.Reg = registry.getOrSet(m.Addr, m.Reg)

	c.OnStartup(func() error { m.Reg = registry.getOrSet(m.Addr, m.Reg); u.Set(m.Addr, m.OnStartup); return nil })
	c.OnRestartFailed(func() error { m.Reg = registry.getOrSet(m.Addr, m.Reg); u.Set(m.Addr, m.OnStartup); return nil })

	c.OnStartup(func() error { return u.ForEach() })
	c.OnRestartFailed(func() error { return u.ForEach() })

	c.OnStartup(func() error {
		conf := dnsserver.GetConfig(c)
		for _, h := range conf.ListenHosts {
			addrstr := conf.Transport + "://" + net.JoinHostPort(h, conf.Port)
			for _, p := range conf.Handlers() {
				vars.PluginEnabled.WithLabelValues(addrstr, conf.Zone, conf.ViewName, p.Name()).Set(1)
			}
		}
		return nil
	})
	c.OnRestartFailed(func() error {
		conf := dnsserver.GetConfig(c)
		for _, h := range conf.ListenHosts {
			addrstr := conf.Transport + "://" + net.JoinHostPort(h, conf.Port)
			for _, p := range conf.Handlers() {
				vars.PluginEnabled.WithLabelValues(addrstr, conf.Zone, conf.ViewName, p.Name()).Set(1)
			}
		}
		return nil
	})

	c.OnRestart(m.OnRestart)
	c.OnRestart(func() error { vars.PluginEnabled.Reset(); return nil })
	c.OnFinalShutdown(m.OnFinalShutdown)

	// Initialize metrics.
	buildInfo.WithLabelValues(coremain.CoreVersion, coremain.GitCommit, runtime.Version()).Set(1)

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		m.Next = next
		return m
	})

	return nil
}

func parse(c *caddy.Controller) (*Metrics, error) {
	met := New(defaultAddr)

	i := 0
	for c.Next() {
		if i > 0 {
			return nil, plugin.ErrOnce
		}
		i++

		zones := plugin.OriginsFromArgsOrServerBlock(nil /* args */, c.ServerBlockKeys)
		for _, z := range zones {
			met.AddZone(z)
		}
		args := c.RemainingArgs()

		switch len(args) {
		case 0:
		case 1:
			met.Addr = args[0]
			_, _, e := net.SplitHostPort(met.Addr)
			if e != nil {
				return met, e
			}
		default:
			return met, c.ArgErr()
		}

		// Parse TLS block if present
		for c.NextBlock() {
			switch c.Val() {
			case "tls":
				if met.tlsConfig != nil {
					return nil, c.Err("tls block already specified")
				}
				tlsCfg := &tlsConfig{
					enabled:        true,
					minVersion:     tls.VersionTLS13,
					clientAuthType: tls.RequireAndVerifyClientCert,
				}

				// Get cert and key files as positional arguments
				args := c.RemainingArgs()
				if len(args) != 2 {
					return nil, c.ArgErr()
				}
				tlsCfg.certFile = args[0]
				tlsCfg.keyFile = args[1]

				// Handle optional tls block parameters
				for c.NextBlock() {
					switch c.Val() {
					case "client_ca":
						if !c.NextArg() {
							return nil, c.ArgErr()
						}
						tlsCfg.clientCAFile = c.Val()
					case "min_version":
						if !c.NextArg() {
							return nil, c.ArgErr()
						}
						ver, err := parseTLSVersion(c.Val())
						if err != nil {
							return nil, c.Err(err.Error())
						}
						tlsCfg.minVersion = ver
					case "client_auth":
						if !c.NextArg() {
							return nil, c.ArgErr()
						}
						authType, err := parseClientAuthType(c.Val())
						if err != nil {
							return nil, c.Err(err.Error())
						}
						tlsCfg.clientAuthType = authType
					default:
						return nil, c.Errf("unknown tls option: %s", c.Val())
					}
				}
				met.tlsConfig = tlsCfg
			default:
				return nil, c.Errf("unknown option: %s", c.Val())
			}
		}
	}
	if err := met.validate(); err != nil {
		return nil, err
	}
	return met, nil
}

func parseTLSVersion(version string) (uint16, error) {
	switch version {
	case "1.3":
		return tls.VersionTLS13, nil
	case "1.2":
		return tls.VersionTLS12, nil
	case "1.1":
		return tls.VersionTLS11, nil
	case "1.0":
		return tls.VersionTLS10, nil
	default:
		return 0, fmt.Errorf("unsupported TLS version: %s", version)
	}
}

func parseClientAuthType(authType string) (tls.ClientAuthType, error) {
	switch strings.ToLower(authType) {
	case "none":
		return tls.NoClientCert, nil
	case "request":
		return tls.RequestClientCert, nil
	case "require":
		return tls.RequireAnyClientCert, nil
	case "verify":
		return tls.VerifyClientCertIfGiven, nil
	case "require_and_verify":
		return tls.RequireAndVerifyClientCert, nil
	default:
		return 0, fmt.Errorf("unsupported client auth type: %s", authType)
	}
}

// defaultAddr is the address the where the metrics are exported by default.
const defaultAddr = "localhost:9153"
