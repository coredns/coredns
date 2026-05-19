//go:build coredns_all || coredns_timeouts

package timeouts

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("timeouts", setup) }
