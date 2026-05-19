//go:build coredns_all || coredns_clouddns

package clouddns

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("clouddns", setup) }
