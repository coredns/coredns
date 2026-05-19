//go:build coredns_all || coredns_loop

package loop

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
