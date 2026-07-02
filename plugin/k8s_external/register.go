//go:build coredns_all || coredns_k8s_external

package external

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register(pluginName, setup) }
