//go:build coredns_all || coredns_debug || coredns_forward || coredns_grpc

package debug

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
