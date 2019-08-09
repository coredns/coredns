package acl

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"unicode"

	"github.com/caddyserver/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/acl/filter"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/miekg/dns"
)

const (
	// QtypeAll is used to match any kinds of DNS records type.
	// NOTE: The value of QtypeAll should be different with other QTYPEs defined in miekg/dns.
	QtypeAll uint16 = dns.TypeANY
)

var (
	// PrivateNets defines all ip addresses reserved for private networks.
	// i.e., 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16.
	PrivateNets = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
)

func init() {
	caddy.RegisterPlugin("acl", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	a, err := parseACL(c)
	if err != nil {
		return err
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

func parseACL(c *caddy.Controller) (acl, error) {
	a := acl{}
	/*
	 * acl [ZONES...] {
	 *   ACTION type QTYPE net SOURCE
	 *   ...
	 * }
	 *
	 * ACTION: allow | block
	 */
	for c.Next() {
		r := Rule{}
		// load <ZONES...>.
		r.Zones = c.RemainingArgs()
		if len(r.Zones) == 0 {
			// if empty, the zones from the configuration block are used.
			r.Zones = make([]string, len(c.ServerBlockKeys))
			copy(r.Zones, c.ServerBlockKeys)
		}
		// strip port and transport.
		for i := range r.Zones {
			r.Zones[i] = plugin.Host(r.Zones[i]).Normalize()
		}

		var err error
		// load all tokens in this block.
		for c.NextBlock() {
			p := Policy{}
			// ACTION type QTYPE net SOURCE
			p.action = strings.ToLower(c.Val())
			if p.action != ALLOW && p.action != BLOCK {
				return a, c.Errf("Unexpected token '%s'; expect '%s' or '%s'", c.Val(), ALLOW, BLOCK)
			}

			if !c.NextArg() {
				return a, c.ArgErr()
			}
			if strings.ToLower(c.Val()) != "type" {
				return a, c.Errf("Unexpected token '%s'; expect 'type'", c.Val())
			}

			if !c.NextArg() {
				return a, c.ArgErr()
			}
			p.qtype, err = parseQype(c.Val())
			if err != nil {
				return a, err
			}

			if !c.NextArg() {
				return a, c.ArgErr()
			}

			var rawNetRanges []string
			sourceType := strings.ToLower(c.Val())
			if sourceType == "net" {
				rawNetRanges = preprocessNetworks(c.RemainingArgs())
			} else if sourceType == "file" {
				if !c.NextArg() {
					return a, c.ArgErr()
				}
				rawNetRanges, err = loadNetworksFromLocalFile(c.Val())
				if err != nil {
					return a, c.Errf("Unable to load networks from local file: %v", err)
				}
			} else {
				return a, c.Errf("Unexpected token '%s'; expect 'net'", c.Val())
			}

			if len(rawNetRanges) == 0 {
				return a, c.Errf("no network is specified")
			}
			var sources []net.IPNet
			for _, rawNet := range rawNetRanges {
				rawNet = normalize(rawNet)
				_, source, err := net.ParseCIDR(rawNet)
				if err != nil {
					return a, c.Errf("Illegal CIDR notation '%s'", rawNet)
				}
				sources = append(sources, *source)
			}
			// TODO: do not hard code 'trie' here. (@ihac)
			p.filter, err = filter.New("trie", sources)
			if err != nil {
				return a, c.Errf("Unable to initialize filter: %v", err)
			}
			r.Policies = append(r.Policies, p)
		}
		a.Rules = append(a.Rules, r)
	}
	return a, nil
}

// normalize appends '/32' for any single ip address.
func normalize(rawNet string) string {
	if idx := strings.IndexAny(rawNet, "/"); idx >= 0 {
		return rawNet
	}
	return rawNet + "/32"
}

// preprocessNetworks replaces all reversed identifier with real subnets or ip addresses.
func preprocessNetworks(rawNets []string) []string {
	var nets []string
	for _, rawNet := range rawNets {
		switch rawNet {
		case "PRIVATE":
			for _, pn := range PrivateNets {
				nets = append(nets, pn)
			}
		case "LOCAL":
			intfs, err := net.Interfaces()
			if err != nil {
				log.Errorf("Failed to get all network interfaces: %v", err)
				continue
			}
			for _, intf := range intfs {
				addrs, err := intf.Addrs()
				if err != nil {
					log.Errorf("Failed to get addresses from interface %s: %v", intf.Name, err)
					continue
				}
				for _, addr := range addrs {
					n := addr.String()
					// ":" should be enough to distinguish IPv4 and IPv6.
					if strings.Contains(n, ":") {
						continue
					}
					nets = append(nets, n)
				}
			}
		case "*":
			fallthrough
		case "ANY":
			return []string{"0.0.0.0/0"}
		default:
			nets = append(nets, rawNet)
		}

	}
	return nets
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

func parseQype(raw string) (uint16, error) {
	switch raw {
	case "A":
		return dns.TypeA, nil
	case "AAAA":
		return dns.TypeAAAA, nil
	case "AFSDB":
		return dns.TypeAFSDB, nil
	case "CAA":
		return dns.TypeCAA, nil
	case "CDNSKEY":
		return dns.TypeCDNSKEY, nil
	case "CDS":
		return dns.TypeCDS, nil
	case "CERT":
		return dns.TypeCERT, nil
	case "CNAME":
		return dns.TypeCNAME, nil
	case "DHCID":
		return dns.TypeDHCID, nil
	case "DLV":
		return dns.TypeDLV, nil
	case "DNAME":
		return dns.TypeDNAME, nil
	case "DNSKEY":
		return dns.TypeDNSKEY, nil
	case "DS":
		return dns.TypeDS, nil
	case "HIP":
		return dns.TypeHIP, nil
	case "KEY":
		return dns.TypeKEY, nil
	case "KX":
		return dns.TypeKX, nil
	case "LOC":
		return dns.TypeLOC, nil
	case "MX":
		return dns.TypeMX, nil
	case "NAPTR":
		return dns.TypeNAPTR, nil
	case "NS":
		return dns.TypeNS, nil
	case "NSEC":
		return dns.TypeNSEC, nil
	case "NSEC3":
		return dns.TypeNSEC3, nil
	case "NSEC3PARAM":
		return dns.TypeNSEC3PARAM, nil
	case "OPENPGPKEY":
		return dns.TypeOPENPGPKEY, nil
	case "PTR":
		return dns.TypePTR, nil
	case "RRSIG":
		return dns.TypeRRSIG, nil
	case "RP":
		return dns.TypeRP, nil
	case "SIG":
		return dns.TypeSIG, nil
	case "SMIMEA":
		return dns.TypeSMIMEA, nil
	case "SOA":
		return dns.TypeSOA, nil
	case "SRV":
		return dns.TypeSRV, nil
	case "SSHFP":
		return dns.TypeSSHFP, nil
	case "TA":
		return dns.TypeTA, nil
	case "TKEY":
		return dns.TypeTKEY, nil
	case "TLSA":
		return dns.TypeTLSA, nil
	case "TSIG":
		return dns.TypeTSIG, nil
	case "TXT":
		return dns.TypeTXT, nil
	case "URI":
		return dns.TypeURI, nil
	case "ANY":
		fallthrough
	case "*":
		return QtypeAll, nil
	default:
		return 0, fmt.Errorf("Unexpected token '%s'; expect legal QTYPE", raw)
	}
}
