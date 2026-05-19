//go:build coredns_all || coredns_file

package file

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("file", setup) }
