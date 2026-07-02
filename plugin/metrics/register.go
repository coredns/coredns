//go:build coredns_all || coredns_prometheus

package metrics

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("prometheus", setup) }
