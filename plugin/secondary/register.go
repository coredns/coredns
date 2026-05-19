//go:build coredns_all || coredns_secondary

package secondary

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("secondary", setup) }
