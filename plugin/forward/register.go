//go:build coredns_all || coredns_forward

package forward

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("forward", setup) }
