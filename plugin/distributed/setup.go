package distributed

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("distributed", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	origin, endpoints, err := distributedParse(c)
	if err != nil {
		return plugin.Error("distributed", err)
	}

	d := &Distributed{
		Entries: &entries{
			byName: map[string][]net.IP{},
			byAddr: map[string]string{},
		},
	}
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		d.Next = next
		d.Origin = origin
		d.Endpoints = &endpoints
		return d
	})

	for i := range endpoints {
		u := endpoints[i]
		c.OnStartup(func() error {
			return u.OnStartup(d)
		})
		c.OnShutdown(func() error {
			return u.OnShutdown(d)
		})
	}

	return nil
}

func distributedParse(c *caddy.Controller) (string, []endpoint, error) {
	for c.Next() {
		args := c.RemainingArgs()
		if len(args) != 2 {
			return "", nil, c.Dispenser.ArgErr()
		}

		origin := plugin.Host(args[1]).Normalize()

		endpoints, err := parseEndpoint(args[0])
		if err != nil {
			return "", nil, err
		}

		if origin == "" || len(endpoints) == 0 {
			return "", nil, c.Dispenser.ArgErr()
		}
		return origin, endpoints, nil
	}
	return "", nil, c.Dispenser.ArgErr()
}

func parseEndpoint(arg string) ([]endpoint, error) {
	var endpoints []endpoint

	entries := map[string]struct{}{}
	for _, s := range strings.Split(arg, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}

		host := s
		port := "53"
		colon := strings.LastIndex(s, ":")
		if colon == len(s)-1 {
			return nil, fmt.Errorf("expecting data after last colon: %q", s)
		}
		if colon != -1 {
			if p, err := strconv.Atoi(s[colon+1:]); err == nil {
				if p < 0 || p >= 65536 {
					return nil, fmt.Errorf("invalid port number: %q", s[colon+1:])
				}
				port = strconv.Itoa(p)
				host = s[:colon]
			}
		}

		slash := strings.LastIndex(host, "/")
		if slash == len(host)-1 {
			return nil, fmt.Errorf("expecting data after last slash: %q", host)
		}
		if slash != -1 {
			ip, ipnet, err := net.ParseCIDR(host)
			if err != nil {
				return nil, fmt.Errorf("invalid cidr block %q: %s", host, err)
			}
			for _, ip = range listIPAddr(ip, ipnet) {
				if _, ok := entries[ip.String()]; !ok {
					entries[ip.String()] = struct{}{}
					endpoints = append(endpoints, endpoint{
						addr: ip.String(),
						port: port,
						quit: make(chan struct{}),
					})
				}
			}

			continue
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return nil, fmt.Errorf("invalid ip address: %q", host)
		}
		if _, ok := entries[ip.String()]; !ok {
			entries[ip.String()] = struct{}{}
			endpoints = append(endpoints, endpoint{
				addr: ip.String(),
				port: port,
				quit: make(chan struct{}),
			})
		}
	}
	return endpoints, nil
}

func listIPAddr(ip net.IP, ipnet *net.IPNet) []net.IP {
	inc := func(ip net.IP) {
		for j := len(ip) - 1; j >= 0; j-- {
			ip[j]++
			if ip[j] > 0 {
				break
			}
		}
	}

	var entries []net.IP
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		entry := make(net.IP, len(ip))
		copy(entry, ip)
		entries = append(entries, entry)
	}
	// remove network address and broadcast address
	return entries[1 : len(entries)-1]
}
