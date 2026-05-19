//go:build coredns_all || coredns_any

package any

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("any", setup) }
