package nomad

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	nomad "github.com/hashicorp/nomad/api"
)

// init registers this plugin.
func init() { plugin.Register(pluginName, setup) }

// setup is the function that gets called when the config parser see the token "nomad". Setup is responsible
// for parsing any extra options the nomad plugin may have. The first token this function sees is "nomad".
func setup(c *caddy.Controller) error {
	n := &Nomad{
		ttl:     uint32(defaultTTL),
		clients: make([]*nomad.Client, 0),
		current: -1,
	}
	if err := parse(c, n); err != nil {
		return plugin.Error("nomad", err)
	}

	c.OnStartup(func() error {
		var err error
		for idx, client := range n.clients {
			_, err := client.Agent().Self()
			if err == nil {
				n.current = idx
				return nil
			}
		}
		return err
	})

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		n.Next = next
		return n
	})

	return nil
}

func parse(c *caddy.Controller, n *Nomad) error {
	var token string
	addresses := []string{} // Multiple addresses are stored here

	for c.Next() {
		for c.NextBlock() {
			selector := strings.ToLower(c.Val())

			switch selector {
			case "address":
				addresses = append(addresses, c.RemainingArgs()[0])
			case "token":
				token = c.RemainingArgs()[0]
			case "zone":
				zone = c.RemainingArgs()[0]
			case "ttl":
				t, err := strconv.Atoi(c.RemainingArgs()[0])
				if err != nil {
					return c.Err("error parsing ttl: " + err.Error())
				}
				if t < 0 || t > 3600 {
					return c.Errf("ttl must be in range [0, 3600]: %d", t)
				}
				n.ttl = uint32(t)
			default:
				return c.Errf("unknown property '%s'", selector)
			}
		}
	}

	// Push an empty address to create a client solely based on the defaults.
	if len(addresses) == 0 {
		addresses = append(addresses, "")
	}

	for _, addr := range addresses {
		cfg := nomad.DefaultConfig()
		if len(addr) > 0 {
			cfg.Address = addr
		}
		if len(token) > 0 {
			cfg.SecretID = token
		}
		client, err := nomad.NewClient(cfg)
		if err != nil {
			return plugin.Error("nomad", err)
		}
		n.clients = append(n.clients, client) // Store all clients
	}

	return nil
}

func (n *Nomad) getClient() (*nomad.Client, error) {
	// Don't bother querying Agent().Self() if there is only one client.
	if len(n.clients) == 1 {
		return n.clients[0], nil
	}
	for i := range len(n.clients) {
		idx := (n.current + i) % len(n.clients)
		_, err := n.clients[idx].Agent().Self()
		if err == nil {
			n.current = idx
			return n.clients[idx], nil
		}
	}
	return nil, fmt.Errorf("no Nomad client available")
}
