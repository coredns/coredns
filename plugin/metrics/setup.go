package metrics

import (
	"crypto/tls"
	"net"
	"runtime"

	"strconv"

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
					clientAuthType: "RequestClientCert",
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
						ver, err := strconv.Atoi(c.Val())
						if err != nil {
							return nil, c.Err(err.Error())
						}
						// Validate TLS version
						switch ver {
						case tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12, tls.VersionTLS13:
							tlsCfg.minVersion = uint16(ver)
						default:
							return nil, c.Errf("invalid TLS version: %d", ver)
						}
					case "client_auth":
						if !c.NextArg() {
							return nil, c.ArgErr()
						}
						// Validate client auth type
						switch c.Val() {
						case "RequestClientCert", "RequireAnyClientCert", "VerifyClientCertIfGiven", "RequireAndVerifyClientCert", "NoClientCert":
							tlsCfg.clientAuthType = c.Val()
						default:
							return nil, c.Errf("invalid client auth type: %s", c.Val())
						}
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

// defaultAddr is the address the where the metrics are exported by default.
const defaultAddr = "localhost:9153"
