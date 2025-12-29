package grpc

import (
	"crypto/tls"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/parse"
	pkgtls "github.com/coredns/coredns/plugin/pkg/tls"
)

func init() { plugin.Register("grpc", setup) }

func setup(c *caddy.Controller) error {
	g, err := parseGRPC(c)
	if err != nil {
		return plugin.Error("grpc", err)
	}

	if g.len() > max {
		return plugin.Error("grpc", fmt.Errorf("more than %d TOs configured: %d", max, g.len()))
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		g.Next = next // Set the Next field, so the plugin chaining works.
		return g
	})

	// Register lifecycle hooks for connection pool management
	c.OnStartup(func() error {
		return g.OnStartup()
	})

	c.OnShutdown(func() error {
		return g.OnShutdown()
	})

	return nil
}

func parseGRPC(c *caddy.Controller) (*GRPC, error) {
	var (
		g   *GRPC
		err error
		i   int
	)
	for c.Next() {
		if i > 0 {
			return nil, plugin.ErrOnce
		}
		i++
		g, err = parseStanza(c)
		if err != nil {
			return nil, err
		}
	}
	return g, nil
}

// defaultProxyOptions returns default configuration for proxies.
func defaultProxyOptions() *proxyOptions {
	return &proxyOptions{
		poolSize: 1,
		expire:   10 * time.Second,
		maxFails: 2,
	}
}

func parseStanza(c *caddy.Controller) (*GRPC, error) {
	g := newGRPC()
	opts := defaultProxyOptions()

	if !c.Args(&g.from) {
		return g, c.ArgErr()
	}
	normalized := plugin.Host(g.from).NormalizeExact()
	if len(normalized) == 0 {
		return g, fmt.Errorf("unable to normalize '%s'", g.from)
	}
	g.from = normalized[0] // only the first is used.

	to := c.RemainingArgs()
	if len(to) == 0 {
		return g, c.ArgErr()
	}

	toHosts, err := parse.HostPortOrFile(to...)
	if err != nil {
		return g, err
	}

	// Parse configuration block before creating proxies
	for c.NextBlock() {
		if err := parseBlock(c, g, opts); err != nil {
			return g, err
		}
	}

	if g.tlsServerName != "" {
		if g.tlsConfig == nil {
			g.tlsConfig = new(tls.Config)
		}
		g.tlsConfig.ServerName = g.tlsServerName
	}

	// Create proxies with parsed options
	for _, host := range toHosts {
		pr, err := newProxy(host, g.tlsConfig, opts)
		if err != nil {
			return nil, err
		}
		g.proxies = append(g.proxies, pr)
	}

	return g, nil
}

func parseBlock(c *caddy.Controller, g *GRPC, opts *proxyOptions) error {
	switch c.Val() {
	case "except":
		ignore := c.RemainingArgs()
		if len(ignore) == 0 {
			return c.ArgErr()
		}
		for i := range ignore {
			g.ignored = append(g.ignored, plugin.Host(ignore[i]).NormalizeExact()...)
		}
	case "tls":
		args := c.RemainingArgs()
		if len(args) > 3 {
			return c.ArgErr()
		}

		for i := range args {
			if !filepath.IsAbs(args[i]) && dnsserver.GetConfig(c).Root != "" {
				args[i] = filepath.Join(dnsserver.GetConfig(c).Root, args[i])
			}
		}
		tlsConfig, err := pkgtls.NewTLSConfigFromArgs(args...)
		if err != nil {
			return err
		}
		g.tlsConfig = tlsConfig
	case "tls_servername":
		if !c.NextArg() {
			return c.ArgErr()
		}
		g.tlsServerName = c.Val()
	case "policy":
		if !c.NextArg() {
			return c.ArgErr()
		}
		switch x := c.Val(); x {
		case "random":
			g.p = &random{}
		case "round_robin":
			g.p = &roundRobin{}
		case "sequential":
			g.p = &sequential{}
		default:
			return c.Errf("unknown policy '%s'", x)
		}
	case "fallthrough":
		g.Fall.SetZonesFromArgs(c.RemainingArgs())

	// New connection pooling and health checking directives
	case "pool_size":
		if !c.NextArg() {
			return c.ArgErr()
		}
		n, err := strconv.Atoi(c.Val())
		if err != nil {
			return err
		}
		if n < 1 {
			return fmt.Errorf("pool_size must be at least 1")
		}
		if n > 100 {
			return fmt.Errorf("pool_size cannot exceed 100")
		}
		opts.poolSize = n
		g.poolSize = n

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
		opts.expire = dur
		g.expire = dur

	case "health_check":
		if !c.NextArg() {
			return c.ArgErr()
		}
		dur, err := time.ParseDuration(c.Val())
		if err != nil {
			return err
		}
		if dur < 0 {
			return fmt.Errorf("health_check can't be negative: %s", dur)
		}
		g.hcInterval = dur

	case "max_fails":
		if !c.NextArg() {
			return c.ArgErr()
		}
		n, err := strconv.ParseUint(c.Val(), 10, 32)
		if err != nil {
			return err
		}
		g.maxFails = uint32(n)
		opts.maxFails = uint32(n)

	default:
		if c.Val() != "}" {
			return c.Errf("unknown property '%s'", c.Val())
		}
	}

	return nil
}

const max = 15 // Maximum number of upstreams.
