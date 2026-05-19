//go:build coredns_all || coredns_geoip

package geoip

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register(pluginName, setup) }
