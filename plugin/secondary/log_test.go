//go:build coredns_all || coredns_secondary

package secondary

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
