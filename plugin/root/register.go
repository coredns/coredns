//go:build coredns_all || coredns_root

package root

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("root", setup) }
