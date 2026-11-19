package log

import (
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/replacer"
	"github.com/coredns/coredns/plugin/pkg/response"

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

func logParse(c *caddy.Controller) ([]Rule, error) {
	var rules []Rule

	for c.Next() {
		args := c.RemainingArgs()
		length := len(rules)

		// The last argument is the format if it contains a placeholder. A name never
		// contains '{', so a lone format is not mistaken for a name.
		format := DefaultLogFormat

		if len(args) > 0 && strings.Contains(args[len(args)-1], "{") {
			format = args[len(args)-1]
			format = strings.ReplaceAll(format, "{common}", CommonLogFormat)
			format = strings.ReplaceAll(format, "{combined}", CombinedLogFormat)
			args = args[:len(args)-1]
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
