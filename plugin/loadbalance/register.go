//go:build coredns_all || coredns_loadbalance

package loadbalance

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("loadbalance", setup) }
