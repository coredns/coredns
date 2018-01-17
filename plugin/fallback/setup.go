package fallback

import (
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"

	"github.com/mholt/caddy"
	"github.com/miekg/dns"
)

func init() {
	caddy.RegisterPlugin("fallback", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	f := Fallback{}

	for c.Next() {
		f.zones = c.RemainingArgs()
		if len(f.zones) == 0 {
			f.zones = make([]string, len(c.ServerBlockKeys))
			copy(f.zones, c.ServerBlockKeys)
		}
		plugin.Zones(f.zones).Normalize()

		for c.NextBlock() {
			switch c.Val() {
			case "on":
				args := c.RemainingArgs()
				if len(args) < 2 {
					return c.Errf("unknown property '%v'", args)
				}
				rcode := 0
				switch args[0] {
				case "SERVFAIL":
					rcode = dns.RcodeServerFailure
				case "NXDOMAIN":
					rcode = dns.RcodeNameError
				case "REFUSED":
					rcode = dns.RcodeRefused
				default:
					return c.Errf("unknown property '%v'", args)
				}
				for _, arg := range args[1:] {
					f.funcs = append(f.funcs, processor{
						rcode:    rcode,
						endpoint: arg,
					})
				}

			default:
				return c.Errf("unknown property '%s'", c.Val())
			}
		}
	}

	c.OnStartup(func() error {
		defaultErrorFunc := dnsserver.DefaultErrorFunc
		dnsserver.DefaultErrorFunc = func(w dns.ResponseWriter, r *dns.Msg, rc int) {
			state := request.Request{W: w, Req: r}
			qname := state.Name()

			zone := plugin.Zones(f.zones).Matches(qname)
			if zone != "" {
				for i := range f.funcs {
					if err := f.funcs[i].ErrorFunc(w, r, rc); err == nil {
						break
					}
				}
			}
			defaultErrorFunc(w, r, rc)
		}
		return nil
	})

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		f.Next = next
		return f
	})

	return nil
}
