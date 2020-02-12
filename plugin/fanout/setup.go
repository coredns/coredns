package fanout

import (
	"fmt"
	"github.com/caddyserver/caddy"
	"github.com/caddyserver/caddy/caddyfile"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/parse"
	pkgtls "github.com/coredns/coredns/plugin/pkg/tls"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/pkg/errors"
	"strconv"
	"strings"
)

func init() {
	caddy.RegisterPlugin("fanout", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	f, err := parseFanout(c)
	if err != nil {
		return plugin.Error("fanout", err)
	}
	l := len(f.clients)
	if len(f.clients) > maxIPCount {
		return plugin.Error("fanout", errors.Errorf("more than %d TOs configured: %d", maxIPCount, l))
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		f.Next = next
		return f
	})

	c.OnStartup(func() error {
		metrics.MustRegister(c, RequestCount, RcodeCount, RequestDuration, HealthcheckFailureCount)
		return f.OnStartup()
	})
	c.OnShutdown(f.OnShutdown)

	return nil
}

// OnStartup starts a goroutines for all clients.
func (f *Fanout) OnStartup() (err error) {
	return nil
}

// OnShutdown stops all configured clients.
func (f *Fanout) OnShutdown() error {
	return nil
}

func parseFanout(c *caddy.Controller) (*Fanout, error) {
	var (
		f   *Fanout
		err error
		i   int
	)
	for c.Next() {
		if i > 0 {
			return nil, plugin.ErrOnce
		}
		i++
		f, err = parsefanoutStanza(&c.Dispenser)
		if err != nil {
			return nil, err
		}
	}

	return f, nil
}

func parsefanoutStanza(c *caddyfile.Dispenser) (*Fanout, error) {
	f := New()
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

	transports := make([]string, len(toHosts))
	for c.NextBlock() {
		err = parseValue(strings.ToLower(c.Val()), f, c)
		if err != nil {
			return nil, err
		}
	}
	for i, host := range toHosts {
		trans, h := parse.Transport(host)
		p := NewClient(h, f.net)
		f.clients = append(f.clients, p)
		transports[i] = trans
	}

	if f.tlsServerName != "" {
		f.tlsConfig.ServerName = f.tlsServerName
	}
	for i := range f.clients {
		if transports[i] == transport.TLS {
			f.clients[i].SetTLSConfig(f.tlsConfig)
		}
	}
	return f, nil
}

func parseValue(v string, f *Fanout, c *caddyfile.Dispenser) (err error) {
	switch v {
	case "tls":
		return parseTLS(f, c)
	case "network":
		return parseProtocol(f, c)
	case "tls-server":
		return parseTLSServer(f, c)
	case "worker-count":
		return parseWorkerCount(f, c)
	case "max-fail-count":
		f.maxFailCount, err = parsePositiveInt(c)
		return err
	case "except":
		return parseIgnored(f, c)
	case "policy":
		return parsePolicy(f, c)
	}
	return err
}

func parseIgnored(f *Fanout, c *caddyfile.Dispenser) error {
	ignore := c.RemainingArgs()
	if len(ignore) == 0 {
		return c.ArgErr()
	}
	for i := 0; i < len(ignore); i++ {
		ignore[i] = plugin.Host(ignore[i]).Normalize()
	}
	f.ignored = ignore
	return nil
}

func parseWorkerCount(f *Fanout, c *caddyfile.Dispenser) error {
	var err error
	f.workerCount, err = parsePositiveInt(c)
	if err == nil {
		if f.workerCount < minWorkerCount {
			return errors.New("worker count should be more or equal 2. Consider to use Forward plugin")
		}
		if f.workerCount > maxWorkerCount {
			return fmt.Errorf("worker count more then max value: %v", maxWorkerCount)
		}
	}
	return err
}

func parsePositiveInt(c *caddyfile.Dispenser) (int, error) {
	if !c.NextArg() {
		return -1, c.ArgErr()
	}
	v := c.Val()
	num, err := strconv.Atoi(v)
	if err != nil {
		return -1, c.ArgErr()
	}
	if num < 0 {
		return -1, c.ArgErr()
	}
	return num, nil
}

func parsePolicy(f *Fanout, c *caddyfile.Dispenser) error {
	if !c.NextArg() {
		return c.ArgErr()
	}
	p := Policy(c.Val())
	f.policy = p
	return p.Validate()
}

func parseTLSServer(f *Fanout, c *caddyfile.Dispenser) error {
	if !c.NextArg() {
		return c.ArgErr()
	}
	f.tlsServerName = c.Val()
	return nil
}

func parseProtocol(f *Fanout, c *caddyfile.Dispenser) error {
	if !c.NextArg() {
		return c.ArgErr()
	}
	if c.Val() != "tcp" && c.Val() != "udp" && c.Val() != "tcp-tls" {
		return errors.New("unknown protocol")
	}
	f.net = strings.ToLower(c.Val())
	return nil
}

func parseTLS(f *Fanout, c *caddyfile.Dispenser) error {
	args := c.RemainingArgs()
	if len(args) > 3 {
		return c.ArgErr()
	}

	tlsConfig, err := pkgtls.NewTLSConfigFromArgs(args...)
	if err != nil {
		return err
	}
	f.tlsConfig = tlsConfig
	return nil
}
