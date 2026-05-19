//go:build coredns_all || coredns_log

package log

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("log", setup) }
