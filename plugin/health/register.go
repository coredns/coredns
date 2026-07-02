//go:build coredns_all || coredns_health

package health

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("health", setup) }
