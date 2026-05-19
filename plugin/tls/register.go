//go:build coredns_all || coredns_tls

package tls

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("tls", setup) }
