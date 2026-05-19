//go:build coredns_all || coredns_grpc_server

package grpc_server

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("grpc_server", setup) }
