//go:build coredns_all || coredns_cache

package cache

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("cache", setup) }
