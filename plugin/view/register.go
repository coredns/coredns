//go:build coredns_all || coredns_view

package view

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("view", setup) }
