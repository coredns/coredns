//go:build coredns_all || coredns_header

package header

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("header", setup) }
