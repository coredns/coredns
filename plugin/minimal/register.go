//go:build coredns_all || coredns_minimal

package minimal

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("minimal", setup) }
