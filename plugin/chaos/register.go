//go:build coredns_all || coredns_chaos

package chaos

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("chaos", setup) }
