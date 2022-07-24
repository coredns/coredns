package tsig

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

func init() {
	caddy.RegisterPlugin(pluginName, caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	t, err := parse(c)
	if err != nil {
		return plugin.Error(pluginName, c.ArgErr())
	}

	config := dnsserver.GetConfig(c)

	config.TsigSecret = t.secrets

	config.AddPlugin(func(next plugin.Handler) plugin.Handler {
		t.Next = next
		return t
	})

	return nil
}

func parse(c *caddy.Controller) (*TSIGServer, error) {
	t := &TSIGServer{
		secrets: make(map[string]string),
		types:   defaultQTypes,
	}

	for i := 0; c.Next(); i++ {
		if i > 0 {
			return nil, plugin.ErrOnce
		}

		t.Zones = plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys)
		for c.NextBlock() {
			switch c.Val() {
			case "secret":
				args := c.RemainingArgs()
				if len(args) != 2 {
					return nil, c.ArgErr()
				}
				k := plugin.Name(args[0]).Normalize()
				if _, exists := t.secrets[k]; exists {
					return nil, fmt.Errorf("key %q redefined", k)
				}
				t.secrets[k] = args[1]
			case "secrets":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				f, err := os.Open(args[0])
				if err != nil {
					return nil, err
				}
				secrets, err := parseKeyFile(f)
				if err != nil {
					return nil, err
				}
				for k, s := range secrets {
					if _, exists := t.secrets[k]; exists {
						return nil, fmt.Errorf("key %q redefined", k)
					}
					t.secrets[k] = s
				}
			case "require":
				t.types = qTypes{}
				args := c.RemainingArgs()
				if len(args) == 0 {
					return nil, c.ArgErr()
				}
				if args[0] == "all" {
					t.all = true
					continue
				}
				if args[0] == "none" {
					continue
				}
				for _, str := range args {
					qt, ok := dns.StringToType[str]
					if !ok {
						return nil, c.Errf("unknown query type '%s'", str)
					}
					t.types[qt] = struct{}{}
				}
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}
	return t, nil
}

func parseKeyFile(f io.Reader) (map[string]string, error) {
	secrets := make(map[string]string)
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) == 0 {
			continue
		}
		if fields[0] != "key" {
			return nil, fmt.Errorf("unexpected token %q", fields[0])
		}
		if len(fields) < 2 {
			return nil, fmt.Errorf("expected key name %q", s.Text())
		}
		key := strings.Trim(fields[1], "\"{")
		if len(key) == 0 {
			return nil, fmt.Errorf("expected key name %q", s.Text())
		}
		key = plugin.Name(key).Normalize()
		if _, ok := secrets[key]; ok {
			return nil, fmt.Errorf("key %q redefined", key)
		}
	key:
		for s.Scan() {
			fields := strings.Fields(s.Text())
			if len(fields) == 0 {
				continue
			}
			switch fields[0] {
			case "algorithm":
				continue
			case "secret":
				if len(fields) < 2 {
					return nil, fmt.Errorf("expected secret key %q", s.Text())
				}
				secret := strings.Trim(fields[1], "\";")
				if len(secret) == 0 {
					return nil, fmt.Errorf("expected secret key %q", s.Text())
				}
				secrets[key] = secret
			case "}":
				fallthrough
			case "};":
				break key
			default:
				return nil, fmt.Errorf("unexpected token %q", fields[0])
			}
		}
		if _, ok := secrets[key]; !ok {
			return nil, fmt.Errorf("expected secret for key %q", key)
		}
	}
	return secrets, nil
}

var defaultQTypes = qTypes{}
