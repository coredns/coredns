//go:build coredns_all || coredns_etcd

package etcd

import (
	"crypto/tls"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	mwtls "github.com/coredns/coredns/plugin/pkg/tls"
	"github.com/coredns/coredns/plugin/pkg/upstream"

	etcdcv3 "go.etcd.io/etcd/client/v3"
)

func setup(c *caddy.Controller) error {
	e, err := etcdParse(c)
	if err != nil {
		return plugin.Error("etcd", err)
	}

	c.OnShutdown(e.OnShutdown)

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		e.Next = next
		return e
	})

	return nil
}

func etcdParse(c *caddy.Controller) (*Etcd, error) {
	config := dnsserver.GetConfig(c)
	etc := Etcd{
		PathPrefix:  "skydns",
		MinLeaseTTL: defaultLeaseMinTTL,
		MaxLeaseTTL: defaultLeaseMaxTTL,
	}
	var (
		tlsConfig *tls.Config
		err       error
		endpoints = []string{defaultEndpoint}
		username  string
		password  string
	)

	etc.Upstream = upstream.New()

	if c.Next() {
		etc.Zones = plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys)
		for c.NextBlock() {
			switch c.Val() {
			case "stubzones":
				// ignored, remove later.
			case "fallthrough":
				etc.Fall.SetZonesFromArgs(c.RemainingArgs())
			case "debug":
				/* it is a noop now */
			case "path":
				if !c.NextArg() {
					return &Etcd{}, c.ArgErr()
				}
				etc.PathPrefix = c.Val()
			case "endpoint":
				args := c.RemainingArgs()
				if len(args) == 0 {
					return &Etcd{}, c.ArgErr()
				}
				endpoints = args
			case "upstream":
				// remove soon
				c.RemainingArgs()
			case "tls": // cert key cacertfile
				args := c.RemainingArgs()
				for i := range args {
					if !filepath.IsAbs(args[i]) && config.Root != "" {
						args[i] = filepath.Join(config.Root, args[i])
					}
				}
				tlsConfig, err = mwtls.NewTLSConfigFromArgs(args...)
				if err != nil {
					return &Etcd{}, err
				}
			case "credentials":
				args := c.RemainingArgs()
				if len(args) == 0 {
					return &Etcd{}, c.ArgErr()
				}
				if len(args) != 2 {
					return &Etcd{}, c.Errf("credentials requires 2 arguments, username and password")
				}
				username, password = args[0], args[1]
			case "min-lease-ttl":
				if !c.NextArg() {
					return &Etcd{}, c.ArgErr()
				}
				minLeaseTTL, err := parseTTL(c.Val())
				if err != nil {
					return &Etcd{}, c.Errf("invalid min-lease-ttl value: %v", err)
				}
				etc.MinLeaseTTL = minLeaseTTL
			case "max-lease-ttl":
				if !c.NextArg() {
					return &Etcd{}, c.ArgErr()
				}
				maxLeaseTTL, err := parseTTL(c.Val())
				if err != nil {
					return &Etcd{}, c.Errf("invalid max-lease-ttl value: %v", err)
				}
				etc.MaxLeaseTTL = maxLeaseTTL
			default:
				if c.Val() != "}" {
					return &Etcd{}, c.Errf("unknown property '%s'", c.Val())
				}
			}
		}
		client, err := newEtcdClient(endpoints, tlsConfig, username, password)
		if err != nil {
			return &Etcd{}, err
		}
		etc.Client = client
		etc.endpoints = endpoints

		return &etc, nil
	}
	return &Etcd{}, nil
}

func newEtcdClient(endpoints []string, cc *tls.Config, username, password string) (*etcdcv3.Client, error) {
	etcdCfg := etcdcv3.Config{
		Endpoints:         endpoints,
		TLS:               cc,
		DialKeepAliveTime: etcdTimeout,
	}
	if username != "" && password != "" {
		etcdCfg.Username = username
		etcdCfg.Password = password
	}
	cli, err := etcdcv3.New(etcdCfg)
	if err != nil {
		return nil, err
	}
	return cli, nil
}

const defaultEndpoint = "http://localhost:2379"

// parseTTL parses a TTL value with flexible time units using Go's standard duration parsing.
// Supports formats like: "30", "30s", "5m", "1h", "90s", "2h30m", etc.
func parseTTL(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	// Handle plain numbers (assume seconds)
	if _, err := strconv.ParseUint(s, 10, 64); err == nil {
		// If it's just a number, append "s" for seconds
		s += "s"
	}

	// Use Go's standard time.ParseDuration for robust parsing
	duration, err := time.ParseDuration(s)
	if err != nil {
		return 0, errors.New("invalid TTL format, use format like '30', '30s', '5m', '1h', or '2h30m'")
	}

	// Convert to seconds and check bounds
	seconds := duration.Seconds()
	if seconds < 0 {
		return 0, errors.New("TTL must be non-negative")
	}
	if seconds > 4294967295 { // uint32 max value
		return 0, errors.New("TTL too large, maximum is 4294967295 seconds")
	}

	return uint32(seconds), nil
}
