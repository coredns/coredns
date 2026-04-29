//go:build coredns_all || coredns_sign

package sign

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("sign", setup) }
