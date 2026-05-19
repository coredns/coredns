//go:build coredns_all || coredns_autopath

package autopath

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("autopath", setup) }
