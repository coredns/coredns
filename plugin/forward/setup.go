package forward

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	pkgtls "github.com/coredns/coredns/plugin/pkg/tls"

	"github.com/mholt/caddy"
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
		once.Do(func() {
			metrics.MustRegister(c, RequestCount, RcodeCount, RequestDuration, HealthcheckFailureCount, SocketGauge)
		})
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
		p.start(f.hcInterval)
	}
	return nil
}

// OnShutdown stops all configured proxies.
func (f *Forward) OnShutdown() error {
	for _, p := range f.proxies {
		p.close()
	}
	return nil
}

// Close is a synonym for OnShutdown().
func (f *Forward) Close() { f.OnShutdown() }

func parseForward(c *caddy.Controller) (*Forward, error) {
	f := New()

	hosts := make([]string, 0)
	tlsHosts := make([]string, 0)

	i := 0
	for c.Next() {
		if i > 0 {
			return nil, plugin.ErrOnce
		}
		i++

		if !c.Args(&f.from) {
			return f, c.ArgErr()
		}
		f.from = plugin.Host(f.from).Normalize()

		to := c.RemainingArgs()
		if len(to) == 0 {
			return f, c.ArgErr()
		}

		for _, h := range to {
			proto, stripped := protocol(h)
			switch proto {
			case DNS:
				// Stripped could either be a regular non-TLS DNS server, or a
				// path to a resolv.conf specifying one or more non-TLS servers.
				// In the latter case the resolv.conf is expanded below by
				// dnsutil.ParseHostPortOrFile.
				hosts = append(hosts, stripped)
			case TLS:
				tlsHosts = append(tlsHosts, stripped)
			default:
				return nil, fmt.Errorf("unknown protocol: %v", h)
			}
		}

		for c.NextBlock() {
			if err := parseBlock(c, f); err != nil {
				return f, err
			}
		}
	}

	if f.tlsServerName != "" {
		f.tlsConfig.ServerName = f.tlsServerName
	}

	hosts, err := dnsutil.ParseHostPortOrFile(hosts...)
	if err != nil {
		return f, err
	}

	tlsHosts, err = dnsutil.ParseHostPortOrFile(tlsHosts...)
	if err != nil {
		return f, err
	}

	for _, h := range hosts {
		p := NewProxy(h, DNS)
		p.SetExpire(f.expire)
		f.proxies = append(f.proxies, p)
	}

	for _, h := range tlsHosts {
		// dnsutil.ParseHostPortOrFile is blissfully unaware of TLS, and thus
		// explicitly sets all ports to 53. If we see a TLS host with port 53 we
		// assume it was changed by dnsutil.ParseHostPortOrFile. We further
		// assume it was originally set to 853 and change it to that port.
		sh, sp, err := net.SplitHostPort(h)
		if err == nil && sp == "53" {
			h = net.JoinHostPort(sh, "853")
		}
		p := NewProxy(h, TLS)
		p.SetTLSConfig(f.tlsConfig)
		p.SetExpire(f.expire)
		f.proxies = append(f.proxies, p)
	}

	return f, nil
}

func parseBlock(c *caddy.Controller, f *Forward) error {
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
	case "force_tcp":
		if c.NextArg() {
			return c.ArgErr()
		}
		f.opts.forceTCP = true
	case "prefer_udp":
		if c.NextArg() {
			return c.ArgErr()
		}
		f.opts.preferUDP = true
	case "tls":
		args := c.RemainingArgs()
		if len(args) > 3 {
			return c.ArgErr()
		}

		tlsConfig, err := pkgtls.NewTLSConfigFromArgs(args...)
		if err != nil {
			return err
		}
		f.tlsConfig = tlsConfig
	case "tls_servername":
		if !c.NextArg() {
			return c.ArgErr()
		}
		f.tlsServerName = c.Val()
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
