//go:build coredns_all || coredns_metadata

package metadata

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("metadata", setup) }
