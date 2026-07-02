//go:build coredns_all || coredns_bufsize

package bufsize

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("bufsize", setup) }
