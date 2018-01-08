package template

import (
	"regexp"
	gotmpl "text/template"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/mholt/caddy"
	"github.com/miekg/dns"
)

func init() {
	caddy.RegisterPlugin("template", caddy.Plugin{
		ServerType: "dns",
		Action:     setupTemplate,
	})
}

func setupTemplate(c *caddy.Controller) error {
	handler, err := templateParse(c)
	if err != nil {
		return plugin.Error("template", err)
	}

	c.OnStartup(OnStartupMetrics)

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		handler.Next = next
		return handler
	})

	return nil
}

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
		handler.Class = class

		if !c.NextArg() {
			return handler, c.ArgErr()
		}
		qtype, ok := dns.StringToType[c.Val()]
		if !ok {
			return handler, c.Errf("invalid RR class %s", c.Val())
		}
		handler.Qtype = qtype

		handler.Zones = c.RemainingArgs()
		if len(handler.Zones) == 0 {
			handler.Zones = make([]string, len(c.ServerBlockKeys))
			copy(handler.Zones, c.ServerBlockKeys)
		}
		for i, str := range handler.Zones {
			handler.Zones[i] = plugin.Host(str).Normalize()
		}

		t := template{}

		t.regex = make([]*regexp.Regexp, 0)
		templatePrefix := ""

		t.answer = make([]*gotmpl.Template, 0)

		for c.NextBlock() {
			switch c.Val() {
			case "match":
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

			case "answer":
				args := c.RemainingArgs()
				if len(args) == 0 {
					return handler, c.ArgErr()
				}
				for _, answer := range args {
					tmpl, err := gotmpl.New("answer").Parse(answer)
					if err != nil {
						return handler, c.Errf("could not compile template: %s, %v", c.Val(), err)
					}
					t.answer = append(t.answer, tmpl)
				}

			case "additional":
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

			case "authority":
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

			case "rcode":
				if !c.NextArg() {
					return handler, c.ArgErr()
				}
				rcode, ok := dns.StringToRcode[c.Val()]
				if !ok {
					return handler, c.Errf("unknown rcode %s", c.Val())
				}
				t.rcode = rcode

			case "fallthrough":
				handler.Fall.SetZonesFromArgs(c.RemainingArgs())

			default:
				return handler, c.ArgErr()
			}
		}

		if len(t.regex) == 0 {
			t.regex = append(t.regex, regexp.MustCompile(".*"))
		}

		if len(t.answer) == 0 && len(t.additional) == 0 && t.rcode == dns.RcodeSuccess {
			return handler, c.Errf("no answer section for template found: %v", handler)
		}

		handler.Templates = append(handler.Templates, t)
	}

	return
}
