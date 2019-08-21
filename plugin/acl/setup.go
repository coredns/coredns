package acl

import (
	"bufio"
	"net"
	"os"
	"strings"
	"unicode"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"

	"github.com/caddyserver/caddy"
	"github.com/infobloxopen/go-trees/iptree"
	"github.com/miekg/dns"
)

const (
	// QtypeAll is used to match any kinds of DNS records type.
	// NOTE: The value of QtypeAll should be different with other QTYPEs defined in miekg/dns.
	QtypeAll uint16 = 299
)

var (
	// DefaultFilter is the default ip filter which matches both IPv4All and IPv6All.
	DefaultFilter *iptree.Tree
)

func init() {
	initDefaultFilter()

	caddy.RegisterPlugin("acl", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func initDefaultFilter() {
	_, IPv4All, _ := net.ParseCIDR("0.0.0.0/0")
	_, IPv6All, _ := net.ParseCIDR("::/0")
	DefaultFilter = iptree.NewTree()
	DefaultFilter.InplaceInsertNet(IPv4All, struct{}{})
	DefaultFilter.InplaceInsertNet(IPv6All, struct{}{})
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

		for c.NextBlock() {
			p := Policy{}

			action := strings.ToLower(c.Val())
			if action == "allow" {
				p.action = ALLOW
			} else if action == "block" {
				p.action = BLOCK
			} else {
				return a, c.Errf("unexpected token '%s'; expect 'allow' or 'block'", c.Val())
			}

			p.qtypes = make(map[uint16]struct{})

			p.filter = iptree.NewTree()

			// match all qtypes and IP addresses.
			if !c.NextArg() {
				p.qtypes[QtypeAll] = struct{}{}
				p.filter = DefaultFilter
				r.Policies = append(r.Policies, p)
				break
			}

			var rawSourceIPRanges []string
			for {
				section := strings.ToLower(c.Val())
				tokens, remaining := loadFollowingTokens(c)
				if len(tokens) == 0 {
					return a, c.Errf("no token specified in '%s' section", section)
				}

				switch section {
				case "type":
					for _, token := range tokens {
						if token == "*" {
							p.qtypes[QtypeAll] = struct{}{}
							break
						}
						qtype, ok := dns.StringToType[token]
						if !ok {
							return a, c.Errf("unexpected token '%s'; expect legal QTYPE", token)
						}
						p.qtypes[qtype] = struct{}{}
					}
				case "net":
					// TODO: refactor to load source IP address one by one.
					for _, token := range tokens {
						rawSourceIPRanges = append(rawSourceIPRanges, token)
					}
				case "file":
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
			if len(p.qtypes) == 0 {
				p.qtypes[QtypeAll] = struct{}{}
			}

			// optional `net` and `file` means all ip addresses.
			if len(rawSourceIPRanges) == 0 {
				p.filter = DefaultFilter
			}

			for _, rawNet := range rawSourceIPRanges {
				if rawNet == "*" {
					p.filter = DefaultFilter
					break
				}
				rawNet = normalize(rawNet)
				_, source, err := net.ParseCIDR(rawNet)
				if err != nil {
					return a, c.Errf("illegal CIDR notation '%s'", rawNet)
				}
				p.filter.InplaceInsertNet(source, struct{}{})
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

// normalize appends '/32' for any single IPv4 address and '/128' for IPv6.
func normalize(rawNet string) string {
	if idx := strings.IndexAny(rawNet, "/"); idx >= 0 {
		return rawNet
	}

	if idx := strings.IndexAny(rawNet, ":"); idx >= 0 {
		return rawNet + "/128"
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
