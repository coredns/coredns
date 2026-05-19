//go:build coredns_all || coredns_grpc

package grpc

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("grpc", setup) }
