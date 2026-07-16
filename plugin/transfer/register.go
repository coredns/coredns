//go:build coredns_all || coredns_transfer

package transfer

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("transfer", setup) }
