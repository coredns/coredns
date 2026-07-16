//go:build coredns_all || coredns_dns64

package dns64

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register(pluginName, setup) }
