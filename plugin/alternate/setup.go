package alternate

import (
	"fmt"
	"strings"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/forward"

	"github.com/mholt/caddy"
	"github.com/miekg/dns"
)

func init() {
	caddy.RegisterPlugin("alternate", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	a := New()

	for c.Next() {
		var (
			original bool
			rcode    string
		)
		if !c.Dispenser.Args(&rcode) {
			return c.ArgErr()
		}
		if rcode == "original" {
			original = true
			// Reread parameter is not rcode. Get it again.
			if !c.Dispenser.Args(&rcode) {
				return c.ArgErr()
			}
		}

		rc, ok := dns.StringToRcode[strings.ToUpper(rcode)]
		if !ok {
			return fmt.Errorf("%s is not a valid rcode", rcode)
		}

		u, err := forward.ParseForwardStanza(&c.Dispenser)
		if err != nil {
			return plugin.Error("alternate", err)
		}

		if _, ok := a.rules[rc]; ok {
			return fmt.Errorf("rcode '%s' is specified more than once", rcode)
		}
		a.rules[rc] = rule{original: original, handler: u}
		if original {
			a.original = true
		}
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		a.Next = next
		return a
	})

	c.OnStartup(func() error {
		for _, r := range a.rules {
			if err := r.handler.OnStartup(); err != nil {
				return err
			}
		}
		return nil
	})

	c.OnShutdown(func() error {
		for _, r := range a.rules {
			if err := r.handler.OnShutdown(); err != nil {
				return err
			}
		}
		return nil
	})

	return nil
}
