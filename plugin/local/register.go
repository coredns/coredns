//go:build coredns_all || coredns_local

package local

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("local", setup) }
