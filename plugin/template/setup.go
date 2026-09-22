package template

import (
	"context"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	gotmpl "text/template"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/upstream"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/builtin"
	"github.com/expr-lang/expr/parser"
	"github.com/miekg/dns"
)

// maxRegexpLen is a hard limit on the length of a regex pattern to prevent
// OOM during regex compilation with malicious input.
const maxRegexpLen = 10000

var varNameRegexp = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func isExprIdentifier(name string) bool {
	tree, err := parser.Parse(name)
	if err != nil {
		return false
	}
	ident, ok := tree.Node.(*ast.IdentifierNode)
	return ok && ident.Value == name
}

func init() { plugin.Register("template", setupTemplate) }

func setupTemplate(c *caddy.Controller) error {
	handler, err := templateParse(c)
	if err != nil {
		return plugin.Error("template", err)
	}

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

		if !c.NextArg() {
			return handler, c.ArgErr()
		}
		qtype, ok := dns.StringToType[c.Val()]
		if !ok {
			return handler, c.Errf("invalid RR class %s", c.Val())
		}

		zones := plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys)
		handler.Zones = append(handler.Zones, zones...)
		t := template{qclass: class, qtype: qtype, zones: zones}

		t.regex = make([]*regexp.Regexp, 0)
		templatePrefix := ""

		t.answer = make([]*gotmpl.Template, 0)
		t.upstream = upstream.New()

		varEnv := exprEnv(context.Background(), nil, &templateData{})

		for c.NextBlock() {
			switch c.Val() {
			case "match":
				args := c.RemainingArgs()
				if len(args) == 0 {
					return handler, c.ArgErr()
				}
				for _, regex := range args {
					if len(regex) > maxRegexpLen {
						return handler, c.Errf("regex pattern too long: %d > %d", len(regex), maxRegexpLen)
					}
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
					tmpl, err := newTemplate("answer", answer)
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
					tmpl, err := newTemplate("additional", additional)
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
					tmpl, err := newTemplate("authority", authority)
					if err != nil {
						return handler, c.Errf("could not compile template: %s, %v\n", c.Val(), err)
					}
					t.authority = append(t.authority, tmpl)
				}

			case "var":
				args := c.RemainingArgs()
				if len(args) < 2 {
					return handler, c.ArgErr()
				}
				if !varNameRegexp.MatchString(args[0]) {
					return handler, c.Errf("invalid variable name %q", args[0])
				}
				_, isEnv := varEnv[args[0]]
				_, isBuiltin := builtin.Index[args[0]]
				if isEnv || isBuiltin || !isExprIdentifier(args[0]) {
					return handler, c.Errf("variable name %q is reserved", args[0])
				}
				prog, err := expr.Compile(strings.Join(args[1:], " "), expr.Env(varEnv), expr.DisableBuiltin("type"))
				if err != nil {
					return handler, c.Errf("could not compile expression: %s, %v", args[0], err)
				}
				if rt := prog.Node().Type(); rt == nil || rt.Kind() == reflect.Interface {
					varEnv[args[0]] = new(any)
				} else {
					varEnv[args[0]] = reflect.Zero(rt).Interface()
				}
				t.vars = append(t.vars, variable{name: args[0], prog: prog})

			case "expr":
				args := c.RemainingArgs()
				if len(args) == 0 {
					return handler, c.ArgErr()
				}
				prog, err := expr.Compile(strings.Join(args, " "), expr.Env(varEnv), expr.DisableBuiltin("type"))
				if err != nil {
					return handler, c.Errf("could not compile expression: %v", err)
				}
				t.exprs = append(t.exprs, prog)

			case "rcode":
				if !c.NextArg() {
					return handler, c.ArgErr()
				}
				rcode, ok := dns.StringToRcode[c.Val()]
				if !ok {
					return handler, c.Errf("unknown rcode %s", c.Val())
				}
				t.rcode = rcode

			case "ederror":
				args := c.RemainingArgs()
				if len(args) != 1 && len(args) != 2 {
					return handler, c.ArgErr()
				}

				code, err := strconv.ParseUint(args[0], 10, 16)
				if err != nil {
					return handler, c.Errf("error parsing extended DNS error code %s, %v\n", c.Val(), err)
				}
				if len(args) == 2 {
					t.ederror = &ederror{code: uint16(code), reason: args[1]}
				} else {
					t.ederror = &ederror{code: uint16(code)}
				}

			case "fallthrough":
				t.fall.SetZonesFromArgs(c.RemainingArgs())

			case "upstream":
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

	return handler, nil
}
