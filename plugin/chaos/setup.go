//go:generate go run owners_generate.go

package chaos

import (
	"sort"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("chaos", setup) }

func setup(c *caddy.Controller) error {
	version, authors, err := parse(c)
	if err != nil {
		return plugin.Error("chaos", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return Chaos{Next: next, Version: version, Authors: authors}
	})

	return nil
}

func parse(c *caddy.Controller) (string, []string, error) {
	// Set here so we pick up AppName and AppVersion that get set in coremain's init().
	chaosVersion = caddy.AppName + "-" + caddy.AppVersion
	version := ""

	if c.Next() {
		args := c.RemainingArgs()
		authors := Owners
		if len(args) == 0 {
			version = trim(chaosVersion)
		} else if len(args) == 1 {
			version = trim(args[0])
		} else {
			version = args[0]
			authorSet := make(map[string]struct{})
			for _, a := range args[1:] {
				authorSet[a] = struct{}{}
			}
			list := make([]string, 0, len(authorSet))
			for k := range authorSet {
				k = trim(k) // limit size to 255 chars
				list = append(list, k)
			}
			sort.Strings(list)
			authors = list
		}

		if c.NextBlock() {
			return "", nil, c.Errf("unknown property '%s'", c.Val())
		}
		return version, authors, nil
	}

	return version, Owners, nil
}

func trim(s string) string {
	if len(s) < 256 {
		return s
	}
	return s[:255]
}

var chaosVersion string
