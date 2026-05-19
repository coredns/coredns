//go:build coredns_all || coredns_nomad

package nomad

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register(pluginName, setup) }
