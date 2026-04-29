//go:build coredns_all || coredns_proxyproto

package proxyproto

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("proxyproto", setup) }
