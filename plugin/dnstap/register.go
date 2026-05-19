//go:build coredns_all || coredns_dnstap

package dnstap

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("dnstap", setup) }
