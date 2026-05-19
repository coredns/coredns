//go:build coredns_all || coredns_rewrite

package rewrite

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("rewrite", setup) }
