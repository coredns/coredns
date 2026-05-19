//go:build coredns_all || coredns_errors

package errors

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("errors", setup) }
