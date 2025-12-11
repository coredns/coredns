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
	config.TsigAlgorithm = t.algorithms

	config.AddPlugin(func(next plugin.Handler) plugin.Handler {
		t.Next = next
		return t
	})

	return nil
}

func parse(c *caddy.Controller) (*TSIGServer, error) {
	t := &TSIGServer{
		secrets:    make(map[string]string),
		algorithms: make(map[string]string),
		types:      defaultQTypes,
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
				if len(args) < 2 {
					return nil, c.ArgErr()
				}
				k := plugin.Name(args[0]).Normalize()
				if _, exists := t.secrets[k]; exists {
					return nil, fmt.Errorf("key %q redefined", k)
				}
				t.secrets[k] = args[1]
				if len(args) > 2 {
					algorithm := plugin.Name(args[2]).Normalize()
					if !validateAlgorithm(algorithm) {
						return nil, c.ArgErr()
					}
					t.algorithms[k] = algorithm
				} else {
					// algorithm recommended by rfc8945
					t.algorithms[k] = dns.HmacSHA256
				}
			case "secrets":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				f, err := os.Open(args[0])
				if err != nil {
					return nil, err
				}
				secrets, algorithms, err := parseKeyFile(f)
				if err != nil {
					return nil, err
				}
				for k, s := range secrets {
					if _, exists := t.secrets[k]; exists {
						return nil, fmt.Errorf("key %q redefined", k)
					}
					t.secrets[k] = s
				}
				for k, s := range algorithms {
					if _, exists := t.algorithms[k]; exists {
						return nil, fmt.Errorf("key %q redefined", k)
					}
					t.algorithms[k] = s
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

func parseKeyFile(f io.Reader) (map[string]string, map[string]string, error) {
	secrets := make(map[string]string)
	algorithms := make(map[string]string)
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) == 0 {
			continue
		}
		if fields[0] != "key" {
			return nil, nil, fmt.Errorf("unexpected token %q", fields[0])
		}
		if len(fields) < 2 {
			return nil, nil, fmt.Errorf("expected key name %q", s.Text())
		}
		key := strings.Trim(fields[1], "\"{")
		if len(key) == 0 {
			return nil, nil, fmt.Errorf("expected key name %q", s.Text())
		}
		key = plugin.Name(key).Normalize()
		if _, ok := secrets[key]; ok {
			return nil, nil, fmt.Errorf("key %q redefined", key)
		}
	key:
		for s.Scan() {
			fields := strings.Fields(s.Text())
			if len(fields) == 0 {
				continue
			}
			switch fields[0] {
			case "algorithm":
				if len(fields) < 2 {
					return nil, nil, fmt.Errorf("expected secret algorithm %q", s.Text())
				}
				algorithm := dns.Fqdn(strings.ToLower(strings.Trim(fields[1], "\";")))
				if len(algorithm) == 0 || !validateAlgorithm(algorithm) {
					return nil, nil, fmt.Errorf("expected secret algorithm %q", s.Text())
				}
				algorithms[key] = algorithm
			case "secret":
				if len(fields) < 2 {
					return nil, nil, fmt.Errorf("expected secret key %q", s.Text())
				}
				secret := strings.Trim(fields[1], "\";")
				if len(secret) == 0 {
					return nil, nil, fmt.Errorf("expected secret key %q", s.Text())
				}
				secrets[key] = secret
			case "}":
				fallthrough
			case "};":
				break key
			default:
				return nil, nil, fmt.Errorf("unexpected token %q", fields[0])
			}
		}
		if _, ok := secrets[key]; !ok {
			return nil, nil, fmt.Errorf("expected secret for key %q", key)
		}
	}
	for key := range secrets {
		if _, ok := algorithms[key]; !ok {
			algorithms[key] = dns.HmacSHA256
		}
	}
	return secrets, algorithms, nil
}

func validateAlgorithm(algorithm string) bool {
	switch algorithm {
	case dns.HmacSHA1, dns.HmacSHA224, dns.HmacSHA256, dns.HmacSHA384, dns.HmacSHA512:
		return true
	default:
		return false
	}
}

var defaultQTypes = qTypes{}
