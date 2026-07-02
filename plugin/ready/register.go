//go:build coredns_all || coredns_ready

package ready

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("ready", setup) }
