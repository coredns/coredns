package sazu

import (
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

func init() { plugin.Register("sazu", setup) }

func setup(c *caddy.Controller) error {
	cfg, err := parseSazu(c)
	if err != nil {
		return plugin.Error("sazu", err)
	}

	config := dnsserver.GetConfig(c)
	// UPDATE is rejected with NotImplemented unless a plugin opts a config
	// in -- this is what lets our own UPDATE handling below actually run.
	config.AllowOpcode(dns.OpcodeUpdate)

	capture := NewRawCapture(5*time.Second, 4096)
	// A signed push carrying real RRSIGs routinely exceeds 512 bytes (RFC
	// 1035's plain-DNS-over-UDP ceiling) -- and, worse, often exceeds the
	// ~1472-byte path MTU before IP fragmentation kicks in, which gets
	// silently dropped by many networks/firewalls entirely (found the
	// hard way against a real server). Raising the UDP receive buffer
	// only helped with the first problem, not the second, so sazuctl
	// sends anything of meaningful size over TCP instead -- both
	// listeners share the same capture, since RawCapture keys entries by
	// address + message ID regardless of transport.
	config.UDPDecorateReaderFunc = capture.DecorateReaderFunc
	config.TCPDecorateReaderFunc = capture.DecorateReaderFunc

	s := &Sazu{
		Zones:                       cfg.zones,
		Validator:                   NewValidator(),
		Capture:                     capture,
		InsecureSkipChainValidation: cfg.insecureSkipChainValidation,
		RequireValidRRSIGs:          cfg.requireValidRRSIGs,
	}

	if cfg.dbPath != "" {
		db, err := Open(cfg.dbPath)
		if err != nil {
			return plugin.Error("sazu", err)
		}
		store, keys, err := db.LoadAll()
		if err != nil {
			db.Close()
			return plugin.Error("sazu", err)
		}
		s.DB = db
		s.Store = store
		s.Keys = keys
		c.OnShutdown(db.Close)
	} else {
		s.Store = NewStore()
		s.Keys = NewKeyRegistry()
	}

	config.AddPlugin(func(next plugin.Handler) plugin.Handler {
		s.Next = next
		return s
	})

	return nil
}

type sazuConfig struct {
	zones                       []string
	insecureSkipChainValidation bool
	requireValidRRSIGs          bool
	dbPath                      string
}

func parseSazu(c *caddy.Controller) (sazuConfig, error) {
	var cfg sazuConfig
	for c.Next() {
		args := c.RemainingArgs()
		cfg.zones = plugin.OriginsFromArgsOrServerBlock(args, c.ServerBlockKeys)

		for c.NextBlock() {
			// RemainingArgs, not NextArg/c.Val(), for both cases below --
			// the idiomatic way elsewhere in this codebase (see e.g.
			// plugin/hosts/setup.go) to collect a directive's own
			// arguments within a block, and to reject the wrong count.
			switch c.Val() {
			case "insecure_skip_chain_validation":
				if len(c.RemainingArgs()) != 0 {
					return sazuConfig{}, c.ArgErr()
				}
				cfg.insecureSkipChainValidation = true
			case "require_valid_rrsigs":
				if len(c.RemainingArgs()) != 0 {
					return sazuConfig{}, c.ArgErr()
				}
				cfg.requireValidRRSIGs = true
			case "db":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return sazuConfig{}, c.ArgErr()
				}
				cfg.dbPath = args[0]
			default:
				return sazuConfig{}, c.ArgErr()
			}
		}
	}
	return cfg, nil
}
