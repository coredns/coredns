//go:build coredns_all || coredns_https3

package https3

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("https3", setup) }
