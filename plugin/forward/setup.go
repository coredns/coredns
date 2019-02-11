package forward

import (
	"fmt"
	"strconv"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	forwardmetrics "github.com/coredns/coredns/plugin/forward/metrics"
	"github.com/coredns/coredns/plugin/forward/proxy"
	"github.com/coredns/coredns/plugin/forward/proxy/dnsproxy"
	"github.com/coredns/coredns/plugin/forward/proxy/grpcproxy"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/parse"
	pkgtls "github.com/coredns/coredns/plugin/pkg/tls"
	"github.com/coredns/coredns/plugin/pkg/transport"

	"github.com/mholt/caddy"
	"github.com/mholt/caddy/caddyfile"
)

func init() {
	caddy.RegisterPlugin("forward", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	f, err := parseForward(c)
	if err != nil {
		return plugin.Error("forward", err)
	}
	if f.Len() > max {
		return plugin.Error("forward", fmt.Errorf("more than %d TOs configured: %d", max, f.Len()))
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		f.Next = next
		return f
	})

	c.OnStartup(func() error {
		metrics.MustRegister(c, fmetrics.RequestCount, fmetrics.RcodeCount, fmetrics.RequestDuration, fmetrics.HealthcheckFailureCount, fmetrics.SocketGauge)
		return f.OnStartup()
	})

	c.OnShutdown(func() error {
		return f.OnShutdown()
	})

	return nil
}

// OnStartup starts a goroutines for all proxies.
func (f *Forward) OnStartup() (err error) {
	for _, p := range f.proxies {
		p.Start(f.hcInterval)
	}
	return nil
}

// OnShutdown stops all configured proxies.
func (f *Forward) OnShutdown() error {
	for _, p := range f.proxies {
		p.Stop()
	}
	return nil
}

// Close is a synonym for OnShutdown().
func (f *Forward) Close() { f.OnShutdown() }

func parseForward(c *caddy.Controller) (*Forward, error) {
	var (
		f   *Forward
		err error
		i   int
	)
	for c.Next() {
		if i > 0 {
			return nil, plugin.ErrOnce
		}
		i++
		f, err = ParseForwardStanza(&c.Dispenser)
		if err != nil {
			return nil, err
		}
	}
	return f, nil
}

// ParseForwardStanza parses one forward stanza
func ParseForwardStanza(c *caddyfile.Dispenser) (*Forward, error) {
	f := New(fmetrics)

	if !c.Args(&f.from) {
		return f, c.ArgErr()
	}
	f.from = plugin.Host(f.from).Normalize()

	to := c.RemainingArgs()
	if len(to) == 0 {
		return f, c.ArgErr()
	}

	toHosts, err := parse.HostPortOrFile(to...)
	if err != nil {
		return f, err
	}

	for c.NextBlock() {
		if err := parseBlock(c, f); err != nil {
			return f, err
		}
	}

	transports := make([]string, len(toHosts))
	var p proxy.Proxy
	for i, host := range toHosts {
		trans, h := parse.Transport(host)
		if f.tlsServerName[trans] != "" {
			f.tlsConfig[trans].ServerName = f.tlsServerName[trans]
		}
		switch trans {
		case transport.DNS:
			opts := &dnsproxy.DNSOpts{
				TLSConfig: f.tlsConfig[transport.DNS],
				Expire:    f.expire,
				ForceTCP:  f.opts.forceTCP,
				PreferUDP: f.opts.preferUDP,
			}
			p = dnsproxy.New(h, opts, fmetrics)
			if err != nil {
				return nil, err
			}
		case transport.GRPC:
			opts := &grpcproxy.GrpcOpts{TLSConfig: f.tlsConfig[trans]}
			p = grpcproxy.New(h, opts, fmetrics)
		}
		f.proxies = append(f.proxies, p)
		transports[i] = trans
	}

	return f, nil
}

func parseBlock(c *caddyfile.Dispenser, f *Forward) error {
	switch c.Val() {
	case "except":
		ignore := c.RemainingArgs()
		if len(ignore) == 0 {
			return c.ArgErr()
		}
		for i := 0; i < len(ignore); i++ {
			ignore[i] = plugin.Host(ignore[i]).Normalize()
		}
		f.ignored = ignore
	case "max_fails":
		if !c.NextArg() {
			return c.ArgErr()
		}
		n, err := strconv.Atoi(c.Val())
		if err != nil {
			return err
		}
		if n < 0 {
			return fmt.Errorf("max_fails can't be negative: %d", n)
		}
		f.maxfails = uint32(n)
	case "health_check":
		if !c.NextArg() {
			return c.ArgErr()
		}
		dur, err := time.ParseDuration(c.Val())
		if err != nil {
			return err
		}
		if dur < 0 {
			return fmt.Errorf("health_check can't be negative: %d", dur)
		}
		f.hcInterval = dur
	case "force_tcp", "dns_force_tcp":
		if c.NextArg() {
			return c.ArgErr()
		}
		f.opts.forceTCP = true
	case "prefer_udp", "dns_prefer_udp":
		if c.NextArg() {
			return c.ArgErr()
		}
		f.opts.preferUDP = true
	case "tls", "dns_tls":
		args := c.RemainingArgs()
		if len(args) > 3 {
			return c.ArgErr()
		}
		tlsConfig, err := pkgtls.NewTLSConfigFromArgs(args...)
		if err != nil {
			return err
		}
		f.tlsConfig[transport.DNS] = tlsConfig
	case "grpc_tls":
		args := c.RemainingArgs()
		if len(args) > 3 {
			return c.ArgErr()
		}
		tlsConfig, err := pkgtls.NewTLSConfigFromArgs(args...)
		if err != nil {
			return err
		}
		f.tlsConfig[transport.GRPC] = tlsConfig
	case "tls_servername", "dns_tls_servername":
		if !c.NextArg() {
			return c.ArgErr()
		}
		f.tlsServerName[transport.DNS] = c.Val()
	case "grpc_tls_servername":
		if !c.NextArg() {
			return c.ArgErr()
		}
		f.tlsServerName[transport.GRPC] = c.Val()
	case "expire":
		if !c.NextArg() {
			return c.ArgErr()
		}
		dur, err := time.ParseDuration(c.Val())
		if err != nil {
			return err
		}
		if dur < 0 {
			return fmt.Errorf("expire can't be negative: %s", dur)
		}
		f.expire = dur
	case "policy":
		if !c.NextArg() {
			return c.ArgErr()
		}
		switch x := c.Val(); x {
		case "random":
			f.p = &random{}
		case "round_robin":
			f.p = &roundRobin{}
		case "sequential":
			f.p = &sequential{}
		default:
			return c.Errf("unknown policy '%s'", x)
		}

	default:
		return c.Errf("unknown property '%s'", c.Val())
	}

	return nil
}

const max = 15 // Maximum number of upstreams.
var fmetrics = forwardmetrics.New()
