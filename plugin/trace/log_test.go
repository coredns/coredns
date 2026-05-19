//go:build coredns_all || coredns_trace

package trace

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
