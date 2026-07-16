//go:build coredns_all || coredns_reload

package reload

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("reload", setup) }
