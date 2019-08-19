package acl

import (
	"bufio"
	"net"
	"os"
	"strings"
	"unicode"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/acl/filter"
	"github.com/coredns/coredns/plugin/metrics"

	"github.com/caddyserver/caddy"
	"github.com/miekg/dns"
)

const (
	// QtypeAll is used to match any kinds of DNS records type.
	// NOTE: The value of QtypeAll should be different with other QTYPEs defined in miekg/dns.
	QtypeAll uint16 = 299
)

var (
	// IPv4All is used to match all IPv4 IP addresses.
	IPv4All *net.IPNet
)

func init() {
	_, IPv4All, _ = net.ParseCIDR("0.0.0.0/0")

	caddy.RegisterPlugin("acl", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	a, err := parse(c)
	if err != nil {
		return plugin.Error("acl", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		a.Next = next
		return a
	})

	// Register all metrics.
	c.OnStartup(func() error {
		metrics.MustRegister(c, RequestBlockCount, RequestAllowCount)
		return nil
	})
	return nil
}

func parse(c *caddy.Controller) (acl, error) {
	a := acl{}
	for c.Next() {
		r := Rule{}
		r.Zones = c.RemainingArgs()
		if len(r.Zones) == 0 {
			// if empty, the zones from the configuration block are used.
			r.Zones = make([]string, len(c.ServerBlockKeys))
			copy(r.Zones, c.ServerBlockKeys)
		}
		for i := range r.Zones {
			r.Zones[i] = plugin.Host(r.Zones[i]).Normalize()
		}

		var err error
		for c.NextBlock() {
			p := Policy{}

			p.action = strings.ToLower(c.Val())
			if p.action != ALLOW && p.action != BLOCK {
				return a, c.Errf("unexpected token '%s'; expect '%s' or '%s'", c.Val(), ALLOW, BLOCK)
			}

			p.qtypes = make(map[uint16]bool)

			p.filter, err = filter.New("trie", nil)
			if err != nil {
				return a, c.Errf("unable to initialize filter: %v", err)
			}

			// match all qtypes and IP addresses.
			if !c.NextArg() {
				p.qtypes[QtypeAll] = true
				p.filter.Add(*IPv4All)
				r.Policies = append(r.Policies, p)
				break
			}

			var rawSourceIPRanges []string
			hasSection := make(map[string]bool)
			for {
				section := strings.ToLower(c.Val())
				tokens, remaining := loadFollowingTokens(c)
				if len(tokens) == 0 {
					return a, c.Errf("no token specified in '%s' section", section)
				}

				switch section {
				case "type":
					hasSection["type"] = true
					for _, token := range tokens {
						if token == "*" {
							p.qtypes[QtypeAll] = true
							continue
						}
						qtype, ok := dns.StringToType[token]
						if !ok {
							return a, c.Errf("unexpected token '%s'; expect legal QTYPE", token)
						}
						p.qtypes[qtype] = true
					}
				case "net":
					hasSection["net"] = true
					// TODO: refactor to load source IP address one by one.
					for _, token := range tokens {
						rawSourceIPRanges = append(rawSourceIPRanges, token)
					}
				case "file":
					hasSection["file"] = true
					for _, token := range tokens {
						ipRanges, err := loadNetworksFromLocalFile(token)
						if err != nil {
							return a, c.Errf("unable to load networks from local file: %v", err)
						}
						for _, ipRange := range ipRanges {
							rawSourceIPRanges = append(rawSourceIPRanges, ipRange)
						}
					}
				default:
					return a, c.Errf("unexpected token '%s'; expect 'type | net | file'", c.Val())
				}
				if !remaining {
					break
				}
			}

			// optional `type` section means all record types.
			if !hasSection["type"] {
				p.qtypes[QtypeAll] = true
			}

			// optional `net` and `file` means all ip addresses.
			if !hasSection["net"] && !hasSection["file"] {
				p.filter.Add(*IPv4All)
			}

			for _, rawNet := range rawSourceIPRanges {
				if rawNet == "*" {
					p.filter.Add(*IPv4All)
					continue
				}
				rawNet = normalize(rawNet)
				_, source, err := net.ParseCIDR(rawNet)
				if err != nil {
					return a, c.Errf("illegal CIDR notation '%s'", rawNet)
				}
				p.filter.Add(*source)
			}
			r.Policies = append(r.Policies, p)
		}
		a.Rules = append(a.Rules, r)
	}
	return a, nil
}

func loadFollowingTokens(c *caddy.Controller) (tokens []string, remain bool) {
	remain = false
	for c.NextArg() {
		token := c.Val()
		identifier := strings.ToLower(token)
		if identifier == "type" || identifier == "net" || identifier == "file" {
			remain = true
			break
		}
		tokens = append(tokens, token)
	}
	return
}

// normalize appends '/32' for any single ip address.
func normalize(rawNet string) string {
	if idx := strings.IndexAny(rawNet, "/"); idx >= 0 {
		return rawNet
	}
	return rawNet + "/32"
}

func loadNetworksFromLocalFile(fileName string) ([]string, error) {
	var nets []string
	file, err := os.Open(fileName)
	defer file.Close()
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := stripComment(scanner.Text())
		// skip empty line.
		if line == "" {
			continue
		}
		nets = append(nets, line)
	}
	return nets, nil
}

// stripComment removes comments.
func stripComment(line string) string {
	commentCh := "#"
	if idx := strings.IndexAny(line, commentCh); idx >= 0 {
		line = line[:idx]
	}
	line = strings.TrimLeftFunc(line, unicode.IsSpace)
	return strings.TrimRightFunc(line, unicode.IsSpace)
}
