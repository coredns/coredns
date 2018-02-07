package bind

import (
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/mholt/caddy/caddyfile"
)

func setupEnhancerBind(dispenser *caddyfile.Dispenser) (dnsserver.KeyEnhancer, error) {
	bke := bindKeyEnhancer{addresses: make([]string, 0)}
	for dispenser.Next() {
		for dispenser.NextArg() {
			if err := bke.addEnhancement(dispenser.Val()); err != nil {
				return nil, plugin.Error("bind", err)
			}
		}
	}
	return bke.EnhanceKey, nil
}
