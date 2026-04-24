//go:build coredns_all || coredns_auto

package auto

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("auto", setup) }
