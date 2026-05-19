//go:build coredns_all || coredns_loop

package loop

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("loop", setup) }
