//go:build coredns_all || coredns_https

package https

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("https", setup) }
