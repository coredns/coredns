package hosts

import (
	"crypto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/mholt/caddy"
)

var log = clog.NewWithPlugin("hosts")

func init() {
	caddy.RegisterPlugin("hosts", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func periodicHostsUpdate(h *Hosts) chan bool {
	parseChan := make(chan bool)

	if *h.hmap.options.reload == durationOf0s {
		return parseChan
	}

	go func() {
		ticker := time.NewTicker(*h.hmap.options.reload)
		for {
			select {
			case <-parseChan:
				return
			case <-ticker.C:
				h.readHosts()
			}
		}
	}()
	return parseChan
}

func setup(c *caddy.Controller) error {
	h, err := hostsParse(c)
	if err != nil {
		return plugin.Error("hosts", err)
	}

	parseChan := periodicHostsUpdate(&h)

	c.OnStartup(func() error {
		h.readHosts()
		return nil
	})

	c.OnShutdown(func() error {
		close(parseChan)
		return nil
	})

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		h.Next = next
		return h
	})

	return nil
}

func hostsParse(c *caddy.Controller) (Hosts, error) {
	config := dnsserver.GetConfig(c)

	options := hostsOptions{
		autoReverse: true,
		encoding:    noEncoding,
		reload:      &durationOf5s,
	}

	h := Hosts{
		Hostsfile: &Hostsfile{
			path: "/etc/hosts",
			hmap: newHostsMap(&options),
		},
	}

	inline := []string{}
	i := 0
	for c.Next() {
		if i > 0 {
			return h, plugin.ErrOnce
		}
		i++

		args := c.RemainingArgs()

		var searchForPredended = (len(args) >= 1)
		for searchForPredended {
			switch args[0] {
			case "no-reverse":
				options.autoReverse = false
			case "md5", "MD5":
				options.encoding = crypto.MD5
				options.autoReverse = false
			case "sha1", "SHA1":
				options.encoding = crypto.SHA1
				options.autoReverse = false
			case "sha224", "SSH224":
				options.encoding = crypto.SHA224
				options.autoReverse = false
			case "sha512", "SSH512":
				options.encoding = crypto.SHA512
				options.autoReverse = false
			default:
				searchForPredended = false
				continue
			}
			args = args[1:]
			searchForPredended = (len(args) >= 1)
		}

		if len(args) >= 1 {
			h.path = args[0]
			args = args[1:]

			if !filepath.IsAbs(h.path) && config.Root != "" {
				h.path = filepath.Join(config.Root, h.path)
			}
			s, err := os.Stat(h.path)
			if err != nil {
				if os.IsNotExist(err) {
					log.Warningf("File does not exist: %s", h.path)
				} else {
					return h, c.Errf("unable to access hosts file '%s': %v", h.path, err)
				}
			}
			if s != nil && s.IsDir() {
				log.Warningf("Hosts file %q is a directory", h.path)
			}
		}

		origins := make([]string, len(c.ServerBlockKeys))
		copy(origins, c.ServerBlockKeys)
		if len(args) > 0 {
			origins = args
		}

		for i := range origins {
			origins[i] = plugin.Host(origins[i]).Normalize()
		}
		h.Origins = origins

		for c.NextBlock() {
			switch c.Val() {
			case "fallthrough":
				h.Fall.SetZonesFromArgs(c.RemainingArgs())
			case "md5", "MD5":
				options.encoding = crypto.MD5
			case "sha1", "SHA1":
				options.encoding = crypto.SHA1
			case "sha224", "SSH224":
				options.encoding = crypto.SHA224
			case "sha512", "SSH512":
				options.encoding = crypto.SHA512
			case "no-reverse":
				options.autoReverse = false
			case "reload":
				duration := c.RemainingArgs()[0]
				if duration == "disabled" {
					options.reload = &durationOf0s
				} else {
					reload, err := time.ParseDuration(duration)
					if err != nil {
						return h, c.Errf("invalid duration for reload '%s'", duration)
					}
					options.reload = &reload
				}
			default:
				if len(h.Fall.Zones) == 0 {
					line := strings.Join(append([]string{c.Val()}, c.RemainingArgs()...), " ")
					inline = append(inline, line)
					continue
				}
				return h, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}

	h.initInline(&options, inline)

	return h, nil
}
