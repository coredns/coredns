//go:build coredns_all || coredns_multisocket

package multisocket

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register(pluginName, setup) }
