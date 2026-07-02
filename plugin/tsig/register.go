//go:build coredns_all || coredns_tsig

package tsig

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register(pluginName, setup) }
