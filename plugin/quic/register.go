//go:build coredns_all || coredns_quic

package quic

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("quic", setup) }
