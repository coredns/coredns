//go:build coredns_all || coredns_debug

package debug

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("debug", setup) }
