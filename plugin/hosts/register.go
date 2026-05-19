//go:build coredns_all || coredns_hosts

package hosts

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("hosts", setup) }
