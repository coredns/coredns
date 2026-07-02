//go:build coredns_all || coredns_pprof

package pprof

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("pprof", setup) }
