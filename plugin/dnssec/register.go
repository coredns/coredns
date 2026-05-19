//go:build coredns_all || coredns_dnssec

package dnssec

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("dnssec", setup) }
