//go:build coredns_all || coredns_nsid

package nsid

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("nsid", setup) }
