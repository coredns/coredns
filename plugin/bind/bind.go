// Package bind allows binding to a specific interface instead of bind to all of them.
package bind

import (
	"fmt"
	"net"

	"github.com/coredns/coredns/core/dnsserver"
)

const bindName = "bind"

func init() {
	// add options to the KeyEnhancermanager
	dnsserver.Register(bindName, setupEnhancerBind)
}

type bindKeyEnhancer struct {
	addresses []string
}

func (bke *bindKeyEnhancer) EnhanceKey(key dnsserver.ZoneAddr) []dnsserver.ZoneAddr {
	// create all needed keys
	newKeys := make([]dnsserver.ZoneAddr, len(bke.addresses))
	for i, addr := range bke.addresses {
		nk := key.Copy()
		nk.CompleteAddress(addr)
		newKeys[i] = nk
	}
	return newKeys
}
func (bke *bindKeyEnhancer) addEnhancement(data string) error {
	ip := net.ParseIP(data)
	if ip == nil {
		return fmt.Errorf("bind : ip value provided is invalid : '%v', it should be an either IPv4 or Ipv6 format", data)
	}
	bke.addresses = append(bke.addresses, data)
	return nil
}
