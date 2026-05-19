//go:build coredns_all || coredns_cancel

package cancel

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("cancel", setup) }
