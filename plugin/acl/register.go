//go:build coredns_all || coredns_acl

package acl

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register(pluginName, setup) }
