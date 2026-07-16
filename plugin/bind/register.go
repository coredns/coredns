//go:build coredns_all || coredns_bind

package bind

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("bind", setup) }
