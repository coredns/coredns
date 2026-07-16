//go:build coredns_all || coredns_dnssec

package dnssec

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
