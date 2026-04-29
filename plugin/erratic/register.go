//go:build coredns_all || coredns_erratic

package erratic

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("erratic", setup) }
