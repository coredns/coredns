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
	zones, insecure, err := parseSazu(c)
	if err != nil {
		return plugin.Error("sazu", err)
	}

	config := dnsserver.GetConfig(c)
	// UPDATE is rejected with NotImplemented unless a plugin opts a config
	// in -- this is what lets our own UPDATE handling below actually run.
	config.AllowOpcode(dns.OpcodeUpdate)

	capture := NewRawCapture(5*time.Second, 4096)
	config.UDPDecorateReaderFunc = capture.DecorateReaderFunc

	s := &Sazu{
		Zones:                       zones,
		Store:                       NewStore(),
		Keys:                        NewKeyRegistry(),
		Validator:                   NewValidator(),
		Capture:                     capture,
		InsecureSkipChainValidation: insecure,
	}

	config.AddPlugin(func(next plugin.Handler) plugin.Handler {
		s.Next = next
		return s
	})

	return nil
}

func parseSazu(c *caddy.Controller) (zones []string, insecureSkipChainValidation bool, err error) {
	for c.Next() {
		args := c.RemainingArgs()
		zones = plugin.OriginsFromArgsOrServerBlock(args, c.ServerBlockKeys)

		for c.NextBlock() {
			switch c.Val() {
			case "insecure_skip_chain_validation":
				if c.NextArg() {
					return nil, false, c.ArgErr()
				}
				insecureSkipChainValidation = true
			default:
				return nil, false, c.ArgErr()
			}
		}
	}
	return zones, insecureSkipChainValidation, nil
}
