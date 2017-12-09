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
	templates, err := templateParse(c)
	if err != nil {
		return plugin.Error("template", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return TemplateHandler{Next: next, Templates: templates}
	})

	return nil
}

func templateParse(c *caddy.Controller) (templates []template, err error) {
	templates = make([]template, 0)

	for c.Next() {
		t := template{}
		if !c.NextArg() {
			return nil, c.ArgErr()
		}
		if class, found := dns.StringToClass[c.Val()]; !found {
			return nil, c.Errf("Invalid query class %s", c.Val())
		} else {
			t.class = class
		}

		if !c.NextArg() {
			return nil, c.ArgErr()
		}
		if queryType, found := dns.StringToType[c.Val()]; !found {
			return nil, c.Errf("Invalid query rr type %s", c.Val())
		} else {
			t.qtype = queryType
		}

		t.regex = make([]*regexp.Regexp, 0)
		templatePrefix := ""

		for _, regex := range c.RemainingArgs() {
			if r, err := regexp.Compile(regex); err != nil {
				return nil, c.Errf("Could not parse regex: %s, %v", regex, err)
			} else {
				templatePrefix = templatePrefix + regex + " "
				t.regex = append(t.regex, r)
			}
		}

		if len(t.regex) == 0 {
			t.regex = append(t.regex, regexp.MustCompile(".*"))
			templatePrefix = ".* "
		}

		t.answer = make([]*gotmpl.Template, 0)

		for c.NextBlock() {
			switch c.Val() {
			case "answer":
				args := c.RemainingArgs()
				if len(args) == 0 {
					return nil, c.ArgErr()
				}
				for _, answer := range args {
					if tmpl, err := gotmpl.New("answer").Parse(answer); err != nil {
						return nil, c.Errf("Could not compile template: %s, %v", c.Val(), err)
					} else {
						t.answer = append(t.answer, tmpl)
					}
				}

			case "additional":
				args := c.RemainingArgs()
				if len(args) == 0 {
					return nil, c.ArgErr()
				}
				for _, additional := range args {
					if tmpl, err := gotmpl.New("additional").Parse(additional); err != nil {
						return nil, c.Errf("Could not compile template: %s, %v\n", c.Val(), err)
					} else {
						t.additional = append(t.additional, tmpl)
					}
				}

			case "rcode":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				if rcode, found := dns.StringToRcode[c.Val()]; !found {
					return nil, c.Errf("Unknown rcode %s", c.Val())
				} else {
					t.rcode = rcode
				}

			default:
				return nil, c.ArgErr()
			}
		}

		if len(t.answer) == 0 && len(t.additional) == 0 && t.rcode == dns.RcodeSuccess {
			return nil, c.Errf("No answer section for template %s %sfound", t.qtype, templatePrefix)
		}

		templates = append(templates, t)
	}

	return templates, nil
}
