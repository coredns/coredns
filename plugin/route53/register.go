//go:build coredns_all || coredns_route53

package route53

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("route53", setup) }
