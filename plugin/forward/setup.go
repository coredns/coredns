package forward

import (
	"fmt"
	"strconv"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dnstap"
	pkgtls "github.com/coredns/coredns/plugin/pkg/tls"
)

func init() { plugin.Register("forward", setup) }

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
		return f.OnStartup()
	})
	c.OnStartup(func() error {
		if taph := dnsserver.GetConfig(c).Handler("dnstap"); taph != nil {
			if tapPlugin, ok := taph.(dnstap.Dnstap); ok {
				f.tapPlugin = &tapPlugin
			}
		}
		return nil
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
		p.stop()
	}
	return nil
}

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
		f, err = parseStanza(c)
		if err != nil {
			return nil, err
		}
	}
	return f, nil
}

func parseStanza(c *caddy.Controller) (*Forward, error) {
	cfg := ForwardConfig{}

	if !c.Args(&cfg.From) {
		return nil, c.ArgErr()
	}

	cfg.To = c.RemainingArgs()
	if len(cfg.To) == 0 {
		return nil, c.ArgErr()
	}

	for c.NextBlock() {
		if err := parseBlock(c, &cfg); err != nil {
			return nil, err
		}
	}

	return NewWithConfig(cfg)
}

func parseBlock(c *caddy.Controller, cfg *ForwardConfig) error {
	switch c.Val() {
	case "except":
		cfg.Except = c.RemainingArgs()
		if len(cfg.Except) == 0 {
			return c.ArgErr()
		}
	case "max_fails":
		if !c.NextArg() {
			return c.ArgErr()
		}
		n, err := strconv.ParseInt(c.Val(), 10, 32)
		if err != nil {
			return err
		}
		if n < 0 {
			return fmt.Errorf("max_fails can't be negative: %d", n)
		}
		maxFails := uint32(n)
		cfg.MaxFails = &maxFails
	case "health_check":
		if !c.NextArg() {
			return c.ArgErr()
		}
		dur, err := time.ParseDuration(c.Val())
		if err != nil {
			return err
		}
		cfg.HealthCheck = &dur

		for c.NextArg() {
			switch hcOpts := c.Val(); hcOpts {
			case "no_rec":
				cfg.HealthCheckNoRec = true
			default:
				return fmt.Errorf("health_check: unknown option %s", hcOpts)
			}
		}

	case "force_tcp":
		if c.NextArg() {
			return c.ArgErr()
		}
		cfg.ForceTCP = true
	case "prefer_udp":
		if c.NextArg() {
			return c.ArgErr()
		}
		cfg.PreferUDP = true
	case "tls":
		args := c.RemainingArgs()
		if len(args) > 3 {
			return c.ArgErr()
		}

		tlsConfig, err := pkgtls.NewTLSConfigFromArgs(args...)
		if err != nil {
			return err
		}
		cfg.TLSConfig = tlsConfig
	case "tls_servername":
		if !c.NextArg() {
			return c.ArgErr()
		}
		cfg.TLSServerName = c.Val()
	case "expire":
		if !c.NextArg() {
			return c.ArgErr()
		}
		dur, err := time.ParseDuration(c.Val())
		if err != nil {
			return err
		}
		cfg.Expire = &dur
	case "policy":
		if !c.NextArg() {
			return c.ArgErr()
		}
		cfg.Policy = c.Val()
	case "max_concurrent":
		if !c.NextArg() {
			return c.ArgErr()
		}
		n, err := strconv.Atoi(c.Val())
		if err != nil {
			return err
		}
		maxConcurrent := int64(n)
		cfg.MaxConcurrent = &maxConcurrent

	default:
		return c.Errf("unknown property '%s'", c.Val())
	}

	return nil
}

const max = 15 // Maximum number of upstreams.
