//go:build coredns_all || coredns_pprof

package pprof

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
