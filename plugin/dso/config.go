package dso

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dso/internal/dsomessage"

	"github.com/miekg/dns"
)

type (
	Config struct {
		// TCPPort is port for DSO services that don't require TLS. Set to 0 to disable.
		TCPPort int

		// TLSPort is port for DSO services that require TLS. Set to 0 to disable.
		TLSPort int

		// InactivityTimeout is maximum amount of time session can be inactive. Clients can further reduce it.
		InactivityTimeout time.Duration

		// KeepAliveInterval is maximum amount of time session can be without any traffic. Clients can further reduce it.
		KeepAliveInterval time.Duration

		// RestartReconnectInterval is RetryDelay interval for gracefully closed sessions due to Restart.
		RestartReconnectInterval time.Duration

		// ShutdownReconnectInterval is RetryDelay interval for gracefully closed sessions due to Shutdown.
		ShutdownReconnectInterval time.Duration

		// TsigSecret is TSIG secrets inherited from [dnsserver.Config] plugin is part of.
		//
		// If non-nil then [Server] will validate requests against their TSIG.
		TsigSecret map[string]string

		// TLSConfig is TLS configuration inherited from [dnsserver.Config] plugin is part of.
		TLSConfig *tls.Config

		// Log is logging configuration. Set to nil to disable.
		Log *LogConfig

		// Push is DSO Push.
		Push *PushConfig
	}

	PushConfig struct {
		// Zones handler is authoritative for.
		//
		// See [Push] for comparison and normalization.
		Zones []string

		// Classes is sorted list of subscribable classes.
		Classes []uint16

		// Types is sorted list of subscribable types.
		Types []uint16

		// RefreshInterval is periodic refresh interval of subscriptions.
		// Set to 0 to disable and use [Server.RefreshPushSubscriptions] instead.
		RefreshInterval time.Duration

		// DebounceDelay is delay to tame burst subscriptions. Set to 0 to disable.
		DebounceDelay time.Duration
	}

	LogConfig struct{}
)

var handlerOnlyConfig = &Config{}

var (
	DefaultTCPPort                   = 0
	DefaultTLSPort                   = 0
	DefaultInactivityTimeout         = 1 * time.Minute
	DefaultKeepAliveInterval         = dsomessage.KeepAliveIntervalRecommended * time.Millisecond
	DefaultRestartReconnectInterval  = 5 * time.Second
	DefaultShutdownReconnectInterval = 15 * time.Second

	DefaultPushRefreshInterval       = 1 * time.Minute
	DefaultPushBurstDebounceInterval = 1 * time.Second
	DefaultPushTypes                 = [...]uint16{
		dns.TypeA,
		dns.TypePTR,
		dns.TypeTXT,
		dns.TypeAAAA,
		dns.TypeSRV,
	}
	DefaultPushClasses = [...]uint16{
		dns.ClassINET,
	}
)

func parsePort(raw string) (port int, err error) {
	port, err = strconv.Atoi(raw)
	if err == nil && port > 65535 {
		err = fmt.Errorf("outside of allowed range")
	}
	if err != nil {
		port, err = net.LookupPort("tcp", raw)
	}
	if err != nil {
		return 0, err
	}
	return port, nil
}

func parseDuration(raw string) (time.Duration, error) {
	d, err := time.ParseDuration(raw)
	if err == nil && d < 0 {
		err = fmt.Errorf("outside of allowed range")
	}
	if err != nil {
		return 0, err
	}
	return d, nil
}

func parseConfig(c *caddy.Controller) (cfg *Config, err error) {
	cfg = &Config{
		TCPPort:                   DefaultTCPPort,
		TLSPort:                   DefaultTLSPort,
		InactivityTimeout:         DefaultInactivityTimeout,
		KeepAliveInterval:         DefaultKeepAliveInterval,
		RestartReconnectInterval:  DefaultRestartReconnectInterval,
		ShutdownReconnectInterval: DefaultShutdownReconnectInterval,
	}
	for i := 0; c.Next(); i++ {
		if i > 0 {
			return nil, plugin.ErrOnce
		}

		args := c.RemainingArgs()
		if len(args) != 0 {
			return nil, c.ArgErr()
		}

		ok := c.NextBlock()
		if !ok {
			return handlerOnlyConfig, nil
		}
		for ok {
			switch c.Val() {
			case "tcp_port":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				port, err := parsePort(args[0])
				if err != nil {
					return nil, c.Errf("invalid tcp_port %q: %v", args[0], err)
				}
				cfg.TCPPort = port
			case "tls_port":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				port, err := parsePort(args[0])
				if err != nil {
					return nil, c.Errf("invalid tls_port %q: %v", args[0], err)
				}
				cfg.TLSPort = port
			case "keepalive":
				args := c.RemainingArgs()
				switch len(args) {
				case 1:
					v, err := parseDuration(args[0])
					if err != nil {
						return nil, c.Errf("invalid keepalive interval %q: %v", args[0], err)
					}
					if v < dsomessage.KeepAliveIntervalMin*time.Millisecond {
						return nil, c.Errf("keepalive interval %q is too short", v)
					}
					cfg.KeepAliveInterval = v
					cfg.InactivityTimeout = v
				case 2:
					v1, err := parseDuration(args[0])
					if err != nil {
						return nil, c.Errf("invalid keepalive interval %q: %v", args[0], err)
					}
					if v1 < dsomessage.KeepAliveIntervalMin*time.Millisecond {
						return nil, c.Errf("keepalive interval %q is too short", v1)
					}
					v2, err := parseDuration(args[1])
					if err != nil {
						return nil, c.Errf("invalid inactivity timeout %q: %v", args[1], err)
					}
					cfg.KeepAliveInterval = v1
					cfg.InactivityTimeout = v2
				default:
					return nil, c.ArgErr()
				}
			case "reconnect":
				args := c.RemainingArgs()
				switch len(args) {
				case 2:
					v1, err := parseDuration(args[0])
					if err != nil {
						return nil, c.Errf("invalid restart interval %q: %v", args[1], err)
					}
					v2, err := parseDuration(args[1])
					if err != nil {
						return nil, c.Errf("invalid shutdown interval %q: %v", args[0], err)
					}
					cfg.RestartReconnectInterval = v1
					cfg.ShutdownReconnectInterval = v2
				default:
					return nil, c.ArgErr()
				}
			case "push":
				cfg.Push = &PushConfig{
					Zones:           plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys),
					Classes:         DefaultPushClasses[:],
					Types:           DefaultPushTypes[:],
					RefreshInterval: DefaultPushRefreshInterval,
					DebounceDelay:   DefaultPushBurstDebounceInterval,
				}
				for c.NextBlock() {
					switch c.Val() {
					case "classes":
						args := c.RemainingArgs()
						if len(args) == 0 {
							return nil, c.ArgErr()
						}
						cfg.Push.Classes = make([]uint16, 0, len(args))
						for _, a := range args {
							cl, ok := dns.StringToClass[a]
							if !ok {
								return nil, c.Errf("unknown DNS class %q", a)
							}
							cfg.Push.Classes = append(cfg.Push.Classes, cl)
						}
					case "types":
						args := c.RemainingArgs()
						if len(args) == 0 {
							return nil, c.ArgErr()
						}
						cfg.Push.Types = make([]uint16, 0, len(args))
						for _, a := range args {
							t, ok := dns.StringToType[a]
							if !ok {
								return nil, c.Errf("unknown DNS type %q", a)
							}
							cfg.Push.Types = append(cfg.Push.Types, t)
						}
					case "refresh":
						args := c.RemainingArgs()
						if len(args) != 1 {
							return nil, c.ArgErr()
						}
						v, err := parseDuration(args[0])
						if err != nil {
							return nil, c.Errf("invalid refresh interval %q: %v", args[0], err)
						}
						cfg.Push.RefreshInterval = v
					case "debounce":
						args := c.RemainingArgs()
						if len(args) != 1 {
							return nil, c.ArgErr()
						}
						v, err := parseDuration(args[0])
						if err != nil {
							return nil, c.Errf("invalid debounce delay %q: %v", args[0], err)
						}
						cfg.Push.DebounceDelay = v
					default:
						return nil, c.Errf("unknown property %q", c.Val())
					}
				}
			case "log":
				cfg.Log = &LogConfig{}
				c.RemainingArgs()
			default:
				return nil, c.Errf("unknown property %q", c.Val())
			}
			ok = c.NextBlock()
		}
	}
	if cfg.TCPPort <= 0 && cfg.TLSPort <= 0 {
		return nil, c.Errf("at least tcp_port or tls_port must be set")
	}
	if cfg.Push != nil && cfg.TLSPort <= 0 {
		return nil, c.Errf("push requires tls_port")
	}

	return cfg, nil
}
