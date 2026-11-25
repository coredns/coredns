package log

import (
	"context"
	"strings"
	"unicode"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/replacer"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func init() { plugin.Register("log", setup) }

func setup(c *caddy.Controller) error {
	rules, err := logParse(c)
	if err != nil {
		return plugin.Error("log", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return Logger{Next: next, Rules: rules, repl: replacer.New()}
	})

	return nil
}

// isLoneFormat reports whether arg, the only argument of a log directive, is a
// format and not a name. Braces are valid in DNS labels (RFC 2181, section 11),
// so an argument with a '{' only counts as a format when it is one of:
//
//   - {common} or {combined}
//   - a single placeholder, such as {remote}
//   - a template with white space (any Unicode white space, including a line
//     break) that contains a placeholder, such as "{remote} {type} {name}"
//
// Anything else, for example svc{name}.example., stays a name. To use such an
// argument as a format, put a name in front of it: log . FORMAT.
func isLoneFormat(arg string) bool {
	// The replacer doesn't know the shortcuts, so swap them for a placeholder it knows.
	arg = strings.NewReplacer("{common}", "{type}", "{combined}", "{type}").Replace(arg)

	// Without a request and a recorder, the replacer turns every placeholder it knows
	// into replacer.EmptyValue and leaves everything else as it is.
	expanded := replacer.New().Replace(context.Background(), request.Request{}, nil, arg)
	switch expanded {
	case arg:
		return false // no placeholder
	case replacer.EmptyValue:
		return true // a single placeholder
	default:
		// A name has no white space. A quoted argument may hold any white space the
		// Caddyfile lexer keeps, such as a line break, so check for all of it.
		return strings.IndexFunc(arg, unicode.IsSpace) >= 0
	}
}

func logParse(c *caddy.Controller) ([]Rule, error) {
	var rules []Rule

	for c.Next() {
		args := c.RemainingArgs()
		length := len(rules)

		// The last argument is the format if it contains a '{'. When it is the only
		// argument it may just as well be a name, so isLoneFormat decides.
		format := DefaultLogFormat

		if n := len(args); n > 0 && strings.Contains(args[n-1], "{") && (n > 1 || isLoneFormat(args[0])) {
			format = args[n-1]
			format = strings.ReplaceAll(format, "{common}", CommonLogFormat)
			format = strings.ReplaceAll(format, "{combined}", CombinedLogFormat)
			args = args[:n-1]
		}

		// No names specified; use the root zone, which matches everything.
		if len(args) == 0 {
			args = []string{"."}
		}

		for _, str := range args {
			rules = append(rules, Rule{
				NameScope: dns.Fqdn(str),
				Format:    format,
				Class:     make(map[response.Class]struct{}),
			})
		}

		// Class refinements in an extra block.
		classes := make(map[response.Class]struct{})
		for c.NextBlock() {
			switch c.Val() {
			// class followed by combinations of all, denial, error and success.
			case "class":
				classesArgs := c.RemainingArgs()
				if len(classesArgs) == 0 {
					return nil, c.ArgErr()
				}
				for _, c := range classesArgs {
					cls, err := response.ClassFromString(c)
					if err != nil {
						return nil, err
					}
					classes[cls] = struct{}{}
				}
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
		if len(classes) == 0 {
			classes[response.All] = struct{}{}
		}

		for i := len(rules) - 1; i >= length; i-- {
			rules[i].Class = classes
		}
	}

	return rules, nil
}
