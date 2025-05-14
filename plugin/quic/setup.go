package quic

import (
	"crypto/tls"
	"fmt"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/parse"
	pkgtls "github.com/coredns/coredns/plugin/pkg/tls"
)

func init() { plugin.Register("quic", setup) }

func setup(c *caddy.Controller) error {
	q, err := ParseQUIC(c)
	if err != nil {
		return plugin.Error("quic", err)
	}

	if q.len() > max {
		return plugin.Error("quic", fmt.Errorf("more than %d TOs configured: %d", max, q.len()))
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		q.Next = next // Set the Next field, so the plugin chaining works.
		return q
	})

	return nil
}

func ParseQUIC(c *caddy.Controller) (*QUIC, error) {
	var (
		q   *QUIC
		err error
		i   int
	)
	for c.Next() {
		if i > 0 {
			return nil, plugin.ErrOnce
		}
		i++
		q, err = parseStanza(c)
		if err != nil {
			return nil, err
		}
	}
	return q, nil
}

func parseStanza(c *caddy.Controller) (*QUIC, error) {
	q := NewQUIC()

	if !c.Args(&q.from) {
		return q, c.ArgErr()
	}
	normalized := plugin.Host(q.from).NormalizeExact()
	if len(normalized) == 0 {
		return q, fmt.Errorf("unable to normalize '%s'", q.from)
	}
	q.from = normalized[0] // only the first is used.

	to := c.RemainingArgs()
	if len(to) == 0 {
		return q, c.ArgErr()
	}

	toHosts, err := parse.HostPortOrFile(to...)
	if err != nil {
		return q, err
	}

	for c.NextBlock() {
		if err := parseBlock(c, q); err != nil {
			return q, err
		}
	}

	if q.tlsConfig == nil {
		q.tlsConfig = new(tls.Config)
	}

	if q.tlsServerName != "" {
		q.tlsConfig.ServerName = q.tlsServerName
	} else {
		q.tlsConfig.InsecureSkipVerify = true
	}

	for _, host := range toHosts {
		pr := newProxy(host, q.tlsConfig)
		if err != nil {
			return nil, err
		}
		q.proxies = append(q.proxies, pr)
	}

	return q, nil
}

func parseBlock(c *caddy.Controller, q *QUIC) error {
	switch c.Val() {
	case "except":
		ignore := c.RemainingArgs()
		if len(ignore) == 0 {
			return c.ArgErr()
		}
		for i := 0; i < len(ignore); i++ {
			q.ignored = append(q.ignored, plugin.Host(ignore[i]).NormalizeExact()...)
		}
	case "tls":
		args := c.RemainingArgs()
		if len(args) > 3 {
			return c.ArgErr()
		}

		tlsConfig, err := pkgtls.NewTLSConfigFromArgs(args...)
		if err != nil {
			return err
		}
		q.tlsConfig = tlsConfig
	case "tls_servername":
		if !c.NextArg() {
			return c.ArgErr()
		}
		q.tlsServerName = c.Val()
	case "policy":
		if !c.NextArg() {
			return c.ArgErr()
		}
		switch x := c.Val(); x {
		case "random":
			q.p = &random{}
		case "round_robin":
			q.p = &roundRobin{}
		case "sequential":
			q.p = &sequential{}
		default:
			return c.Errf("unknown policy '%s'", x)
		}
	default:
		if c.Val() != "}" {
			return c.Errf("unknown property '%s'", c.Val())
		}
	}

	return nil
}

const max = 15 // Maximum number of upstreams.
