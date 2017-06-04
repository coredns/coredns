package hosts

import (
	"log"
	"os"
	"path"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/middleware"

	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("hosts", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	a, err := hostsParse(c)
	if err != nil {
		return middleware.Error("hosts", err)
	}

	dnsserver.GetConfig(c).AddMiddleware(func(next middleware.Handler) middleware.Handler {
		a.Next = next
		return a
	})

	return nil
}

func hostsParse(c *caddy.Controller) (Hosts, error) {
	var a = Hosts{
		Hostsfile: &Hostsfile{path: "/etc/hosts"},
	}

	config := dnsserver.GetConfig(c)

	for c.Next() {
		if c.Val() == "hosts" {
			for c.NextBlock() {
				switch c.Val() {
				case "file": // file HOSTSFILE
					if !c.NextArg() {
						return a, c.ArgErr()
					}
					a.path = c.Val()
					if !path.IsAbs(a.path) && config.Root != "" {
						a.path = path.Join(config.Root, a.path)
					}
					_, err := os.Stat(a.path)
					if err != nil {
						if os.IsNotExist(err) {
							log.Printf("[WARNING] File does not exist: %s", a.path)
						} else {
							return a, c.Errf("Unable to access hosts file '%s': %v", a.path, err)
						}
					}
				}
			}

		}
	}
	a.ReadHosts()
	return a, nil
}
