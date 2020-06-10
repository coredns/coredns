package transfer

import (
	"errors"
	"fmt"
	"net"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	parsepkg "github.com/coredns/coredns/plugin/pkg/parse"
	"github.com/coredns/coredns/plugin/pkg/transport"

	"github.com/caddyserver/caddy"
)

func init() {
	caddy.RegisterPlugin("transfer", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	t, err := parse(c)

	if err != nil {
		return plugin.Error("transfer", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		t.Next = next
		return t
	})

	c.OnStartup(func() error {
		// find all plugins that implement Transferer and add them to Transferers
		plugins := dnsserver.GetConfig(c).Handlers()
		for _, pl := range plugins {
			tr, ok := pl.(Transferer)
			if !ok {
				continue
			}
			t.Transferers = append(t.Transferers, tr)
			ch, st := tr.Notify()
			if ch != nil && st != nil {
				go t.Notify(ch, st)
			}
		}
		return nil
	})

	return nil
}

func parse(c *caddy.Controller) (*Transfer, error) {

	t := &Transfer{}
	for c.Next() {
		x := &xfr{to: make(hosts)}
		zones := c.RemainingArgs()

		if len(zones) != 0 {
			x.Zones = zones
			for i := 0; i < len(x.Zones); i++ {
				nzone, err := plugin.Host(x.Zones[i]).MustNormalize()
				if err != nil {
					return nil, err
				}
				x.Zones[i] = nzone
			}
		} else {
			x.Zones = make([]string, len(c.ServerBlockKeys))
			for i := 0; i < len(c.ServerBlockKeys); i++ {
				nzone, err := plugin.Host(c.ServerBlockKeys[i]).MustNormalize()
				if err != nil {
					return nil, err
				}
				x.Zones[i] = nzone
			}
		}

		for c.NextBlock() {
			switch c.Val() {
			case "to":
				host, nOpts, err := parseTo(c.RemainingArgs())
				if err != nil {
					return nil, err
				}
				x.to[host] = nOpts
			default:
				return nil, plugin.Error("transfer", c.Errf("unknown property '%s'", c.Val()))
			}
		}
		if len(x.to) == 0 {
			return nil, plugin.Error("transfer", c.Err("'to' is required"))
		}
		t.xfrs = append(t.xfrs, x)
	}
	return t, nil
}

func parseTo(args []string) (host string, nOpts *notifyOpts, err error) {
	if len(args) == 0 {
		return host, nOpts, errors.New("expected host")
	}
	host = args[0]
	if host == "*" {
		return host, nOpts, err
	}
	host, err = parsepkg.HostPort(host, transport.Port)
	if err != nil {
		return host, nOpts, err
	}
	if len(args) == 1 {
		return host, nOpts, err
	}
	if len(args) > 1 && args[1] != "notify" {
		return host, nOpts, fmt.Errorf("invalid option %v", args[1])
	}
	nOpts = &notifyOpts{}

	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "source":
			i++
			if i > len(args) {
				return host, nOpts, errors.New("expected source address")
			}
			if ip := net.ParseIP(args[i]); ip != nil {
				nOpts.source = &net.UDPAddr{IP: ip}
			} else {
				return host, nOpts, fmt.Errorf("invalid source address '%v'", args[i])
			}
		default:
			return host, nOpts, fmt.Errorf("invalid option %v", args[i])
		}
	}
	return host, nOpts, nil
}
