//go:build coredns_all || coredns_azure

package azure

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("azure", setup) }
