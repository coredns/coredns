//go:build coredns_all || coredns_trace

package trace

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("trace", setup) }
