//go:build coredns_all || coredns_kubernetes

package kubernetes

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register(pluginName, setup) }
