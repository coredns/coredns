//go:build coredns_all || coredns_dnstap || coredns_forward

package dnstap

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
