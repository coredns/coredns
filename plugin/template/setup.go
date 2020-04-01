package template

import (
	"regexp"
	gotmpl "text/template"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/upstream"

	"github.com/caddyserver/caddy"
	"github.com/miekg/dns"
)

const pluginName = "template"

func init() { plugin.Register(pluginName, setupTemplate) }

func setupTemplate(c *caddy.Controller) error {
	handler, err := templateParse(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}

	if err := setupMetrics(c); err != nil {
		return plugin.Error(pluginName, err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		handler.Next = next
		return handler
	})

	return nil
}

const (
	templateMatch       = "match"
	templateAnswer      = "answer"
	templateAdditional  = "additional"
	templateAuthority   = "authority"
	templateRcode       = "rcode"
	templateFallthrough = "fallthrough"
	templateUpstream    = "upstream"
)

func templateParse(c *caddy.Controller) (handler Handler, err error) {
	handler.Templates = make([]template, 0)

	for c.Next() {

		if !c.NextArg() {
			return handler, c.ArgErr()
		}
		class, ok := dns.StringToClass[c.Val()]
		if !ok {
			return handler, c.Errf("invalid query class %s", c.Val())
		}

		if !c.NextArg() {
			return handler, c.ArgErr()
		}
		qtype, ok := dns.StringToType[c.Val()]
		if !ok {
			return handler, c.Errf("invalid RR class %s", c.Val())
		}

		zones := c.RemainingArgs()
		if len(zones) == 0 {
			zones = make([]string, len(c.ServerBlockKeys))
			copy(zones, c.ServerBlockKeys)
		}
		for i, str := range zones {
			zones[i] = plugin.Host(str).Normalize()
		}
		handler.Zones = append(handler.Zones, zones...)

		t := template{qclass: class, qtype: qtype, zones: zones}

		t.regex = make([]*regexp.Regexp, 0)
		templatePrefix := ""

		t.answer = make([]*gotmpl.Template, 0)
		t.upstream = upstream.New()

		for c.NextBlock() {
			switch c.Val() {
			case templateMatch:
				args := c.RemainingArgs()
				if len(args) == 0 {
					return handler, c.ArgErr()
				}
				for _, regex := range args {
					r, err := regexp.Compile(regex)
					if err != nil {
						return handler, c.Errf("could not parse regex: %s, %v", regex, err)
					}
					templatePrefix = templatePrefix + regex + " "
					t.regex = append(t.regex, r)
				}

			case templateAnswer:
				args := c.RemainingArgs()
				if len(args) == 0 {
					return handler, c.ArgErr()
				}
				for _, answer := range args {
					tmpl, err := gotmpl.New(templateAnswer).Parse(answer)
					if err != nil {
						return handler, c.Errf("could not compile template: %s, %v", c.Val(), err)
					}
					t.answer = append(t.answer, tmpl)
				}

			case templateAdditional:
				args := c.RemainingArgs()
				if len(args) == 0 {
					return handler, c.ArgErr()
				}
				for _, additional := range args {
					tmpl, err := gotmpl.New("additional").Parse(additional)
					if err != nil {
						return handler, c.Errf("could not compile template: %s, %v\n", c.Val(), err)
					}
					t.additional = append(t.additional, tmpl)
				}

			case templateAuthority:
				args := c.RemainingArgs()
				if len(args) == 0 {
					return handler, c.ArgErr()
				}
				for _, authority := range args {
					tmpl, err := gotmpl.New("authority").Parse(authority)
					if err != nil {
						return handler, c.Errf("could not compile template: %s, %v\n", c.Val(), err)
					}
					t.authority = append(t.authority, tmpl)
				}

			case templateRcode:
				if !c.NextArg() {
					return handler, c.ArgErr()
				}
				rcode, ok := dns.StringToRcode[c.Val()]
				if !ok {
					return handler, c.Errf("unknown rcode %s", c.Val())
				}
				t.rcode = rcode

			case templateFallthrough:
				t.fall.SetZonesFromArgs(c.RemainingArgs())

			case templateUpstream:
				// remove soon
				c.RemainingArgs()
			default:
				return handler, c.ArgErr()
			}
		}

		if len(t.regex) == 0 {
			t.regex = append(t.regex, regexp.MustCompile(".*"))
		}

		handler.Templates = append(handler.Templates, t)
	}

	return
}
