package loop

import (
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"

	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("loop", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	l, err := parse(c)
	if err != nil {
		return plugin.Error("loop", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		l.Next = next
		return l
	})

	// Send query to ourselves and see if it end up with us again.
	c.OnStartup(func() error {
		// Another Go function, otherwise we block startup and can't send the packet.
		go func() {
			deadline := time.Now().Add(30 * time.Second)
			conf := dnsserver.GetConfig(c)

			for time.Now().Before(deadline) {
				ok := 0
				for _, lh := range conf.ListenHosts {
					addr := net.JoinHostPort(lh, conf.Port)
					if _, err := l.exchange(addr); err != nil {
						continue
					}
					ok++
				}

				if ok == len(conf.ListenHosts) {
					go func() {
						time.Sleep(2 * time.Second)
						l.setDisabled()
					}()
					return
				}
				time.Sleep(2 * time.Second)
			}
			l.setDisabled()
		}()
		return nil
	})

	return nil
}

func parse(c *caddy.Controller) (*Loop, error) {
	_ = c.NextArg()
	if c.NextArg() {
		return nil, c.ArgErr()
	}

	zone := "."
	if len(c.ServerBlockKeys) > 0 {
		zone = plugin.Host(c.ServerBlockKeys[0]).Normalize()
	}

	return New(zone), nil
}

// qname returns a random name. <rand.Int()>.<rand.Int().<l.zone>.
func qname(zone string) string {
	l1 := strconv.Itoa(r.Int())
	l2 := strconv.Itoa(r.Int())

	return dnsutil.Join([]string{l1, l2, zone})
}

var r = rand.New(rand.NewSource(time.Now().UnixNano()))
